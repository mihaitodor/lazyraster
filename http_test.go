package main

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Nitro/filecache"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	didDownload         bool
	downloadShouldSleep bool
	downloadShouldError bool
	downloadCount       int
	countLock           sync.Mutex

	mockDownloader = func(fname string, localPath string) error {
		if downloadShouldError {
			return errors.New("Oh no! Tragedy!")
		}
		if downloadShouldSleep {
			time.Sleep(10 * time.Millisecond)
		}
		countLock.Lock()
		downloadCount += 1
		countLock.Unlock()
		didDownload = true
		return nil
	}
)

// CopyFile copies the contents from src to dst using io.Copy.
// If dst does not exist, CopyFile creates it with permissions perm;
// otherwise CopyFile truncates it before writing.
func CopyFile(dst, src string, perm os.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()
	_, err = io.Copy(out, in)
	return
}

func Test_urlToFilename(t *testing.T) {
	Convey("urlToFilename()", t, func() {
		Convey("Strips leading '/documents'", func() {
			fn := urlToFilename("/documents/testing-bucket/foo-file.pdf")

			So(fn, ShouldNotContainSubstring, "/documents")
		})

		// TODO This is temporary! Remove when migrated.
		Convey("Strips the bucket name from the path", func() {
			fn := urlToFilename("/documents/testing-bucket/foo-file.pdf")
			So(fn, ShouldNotContainSubstring, "/testing-bucket")
		})

		Convey("Does not return a leading slash", func() {
			fn := urlToFilename("/documents/testing-bucket/foo-file.pdf")
			So(fn, ShouldEqual, "foo-file.pdf")
		})
	})
}

func Test_EndToEnd(t *testing.T) {
	Convey("End-to-end testing handleImage()", t, func() {
		didDownload = false
		downloadCount = 0

		cache, _ := filecache.NewS3Cache(10, os.TempDir(), "aragorn-foo", "gondor-north-1")
		cache.DownloadFunc = mockDownloader

		rasterCache, _ := NewRasterCache(1)

		h := &RasterHttpServer{
			cache:       cache,
			ring:        nil, // Don't test with ringman for now
			rasterCache: rasterCache,
			urlSecret:   "",
			agent:       nil,
		}

		filename := "73/069741a92a2f641eb428ba6d12ccb9af" // cache file for sample.pdf

		Reset(func() {
			os.Remove(cache.GetFileName("sample.pdf"))
		})

		Convey("Handling error conditions", func() {
			Convey("When no page is specified", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/asdf.pdf", nil)
				recorder := httptest.NewRecorder()

				h.handleImage(recorder, req)
				So(recorder.Result().StatusCode, ShouldEqual, 400)
			})

			Convey("When the page is not contained in the document", func() {
				os.MkdirAll(filepath.Join(os.TempDir(), filepath.Dir(filename)), 0755)
				CopyFile(cache.GetFileName("sample.pdf"), "fixtures/sample.pdf", 0644)

				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=10", nil)
				recorder := httptest.NewRecorder()

				h.handleImage(recorder, req)
				So(recorder.Result().StatusCode, ShouldEqual, 404)
			})

			Convey("When the page is not valid", func() {
				os.MkdirAll(filepath.Join(os.TempDir(), filepath.Dir(filename)), 0755)
				CopyFile(cache.GetFileName("sample.pdf"), "fixtures/sample.pdf", 0644)

				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=-1", nil)
				recorder := httptest.NewRecorder()

				h.handleImage(recorder, req)
				So(recorder.Result().StatusCode, ShouldEqual, 400)
			})

			Convey("When file is not present", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/asdf.pdf", nil)
				recorder := httptest.NewRecorder()
				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(recorder.Result().StatusCode, ShouldEqual, 400)
				So(string(body), ShouldContainSubstring, "Invalid page")
			})

			Convey("Rejects badly/unsigned URLs when signing is required", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=1", nil)
				recorder := httptest.NewRecorder()
				h.urlSecret = "secret"

				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(recorder.Result().StatusCode, ShouldEqual, 403)
				So(string(body), ShouldContainSubstring, "Invalid signature")
			})
		})

		Convey("When everything is working", func() {
				os.MkdirAll(filepath.Join(os.TempDir(), filepath.Dir(filename)), 0755)
				CopyFile(cache.GetFileName("sample.pdf"), "fixtures/sample.pdf", 0644)
				recorder := httptest.NewRecorder()

			Convey("Handles a normal request", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=1", nil)

				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(len(body), ShouldBeGreaterThan, 1024)
				So(recorder.Result().StatusCode, ShouldEqual, 200)
				So(didDownload, ShouldBeTrue)
				So(downloadCount, ShouldEqual, 1)
			})

			Convey("Handles a jpegs", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=1&width=1024&quality=75&imageType=image/jpeg", nil)

				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(len(body), ShouldBeGreaterThan, 1024)
				So(recorder.Result().StatusCode, ShouldEqual, 200)
				So(recorder.Result().Header["Content-Type"][0], ShouldEqual, "image/jpeg")
			})

			Convey("Handles a pngs", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=1&width=1024&quality=75&imageType=image/png", nil)

				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(len(body), ShouldBeGreaterThan, 1024)
				So(recorder.Result().StatusCode, ShouldEqual, 200)
				So(recorder.Result().Header["Content-Type"][0], ShouldEqual, "image/png")
			})

			Convey("Handles a bunch of options", func() {
				req := httptest.NewRequest("GET", "/documents/somewhere/sample.pdf?page=1&width=1024&quality=75", nil)

				h.handleImage(recorder, req)

				body, err := ioutil.ReadAll(recorder.Result().Body)
				So(err, ShouldBeNil)
				So(len(body), ShouldBeGreaterThan, 1024)
				So(recorder.Result().StatusCode, ShouldEqual, 200)
			})
		})
	})
}
