package main

import (
	"fmt"

	"github.com/Nitro/filecache"
	"github.com/Nitro/ringman"
	log "github.com/Sirupsen/logrus"
	"github.com/newrelic/go-agent"
	"github.com/kelseyhightower/envconfig"
	"github.com/relistan/rubberneck"
)

const (
	ImageMaxWidth = 4096
)

type Config struct {
	BaseDir         string   `envconfig:"BASE_DIR" default:"."`
	Port            string   `envconfig:"PORT" default:"8000"`
	AwsRegion       string   `envconfig:"AWS_REGION" default:"us-west-1"`
	S3Bucket        string   `envconfig:"S3_BUCKET" default:"nitro-junk"`
	ClusterSeeds    []string `envconfig:"CLUSTER_SEEDS" default:"127.0.0.1"`
	CacheSize       int      `envconfig:"CACHE_SIZE" default:"512"`
	RedisPort       int      `envconfig:"REDIS_PORT" default:"6379"`
	NewRelicLicense string   `envconfig:"NEWRELIC_LICENSE_KEY"`
	NewRelicAppName string   `envconfig:"NEWRELIC_APP_NAME" default:"raster-service dev"`
}

func configureNewRelic(config *Config) newrelic.Application {
	if config.NewRelicLicense == "" {
		return &NewRelicStubApplication{}
	}

	nrApp, err := newrelic.NewApplication(
		newrelic.NewConfig(config.NewRelicAppName, config.NewRelicLicense),
	)
	if err != nil {
		log.Fatalf("Unable to create New Relic application: %s", err)
	}

	return nrApp
}

func main() {
	log.SetLevel(log.DebugLevel)

	var config Config

	envconfig.Process("raster", &config)
	rubberneck.NewPrinter(log.Infof, rubberneck.NoAddLineFeed).Print(config)

	nrApp := configureNewRelic(&config)

	cache, err := filecache.NewS3Cache(512, config.BaseDir, config.S3Bucket, config.AwsRegion)
	if err != nil {
		log.Fatalf("Unable to create LRU cache: %s", err)
	}

	ring, err := ringman.NewDefaultMemberlistRing(config.ClusterSeeds, config.Port)
	if err != nil {
		log.Fatalf("Unble to establish memberlist ring: %s", err)
	}

	rasterCache, err := NewDefaultRasterCache()
	if err != nil {
		log.Fatalf("Unble to initialize the rasterizer cache: %s", err)
	}

	// Run the Redis protocol server and wire it up to our hash ring
	go func() {
		err := serveRedis(fmt.Sprintf(":%d", config.RedisPort), ring.Manager)
		if err != nil {
			log.Fatalf("Error starting Redis protocol server: %s", err)
		}
	}()

	err = serveHttp(&config, cache, ring, rasterCache, &nrApp)
	if err != nil {
		panic(err.Error())
	}
}
