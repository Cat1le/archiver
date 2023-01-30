package main

import (
	"archiver/storage"
	"bytes"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/utils"
	"mime"
	"mime/multipart"
	"net/url"
)

const (
	FileMaxSize         = 1024 * 1024 * 1024
	FileMaxSizeReadable = "1GB"
	StorageDirectory    = "files"
)

func main() {
	app := fiber.New(fiber.Config{Immutable: true, BodyLimit: FileMaxSize * 2})
	app.Use(cors.New(cors.Config{AllowCredentials: true}))
	app.Use(logger.New())
	storageFactory := storage.New(StorageDirectory)
	app.Get("/begin", func(ctx *fiber.Ctx) error {
		return ctx.SendString(utils.UUIDv4())
	})
	app.Post("/upload/:id", func(ctx *fiber.Ctx) error {
		store := storageFactory.Session(ctx.Params("id", ""))
		if store.StatusCode() != storage.CodeWaiting {
			return errors.New("archive already processing")
		}
		form, err := readForm(ctx)
		if err != nil {
			return err
		}
		file, err := extractFile(form)
		if err != nil {
			return err
		}
		if file.Size > FileMaxSize {
			return errors.New("file size is bigger than " + FileMaxSizeReadable)
		}
		store.Create(file)
		err = ctx.SendString("Successfully uploaded")
		if err != nil {
			panic(err)
		}
		return nil
	})
	app.Get("/delete/:id/:file", func(ctx *fiber.Ctx) error {
		store := storageFactory.Session(ctx.Params("id", ""))
		if store.StatusCode() != storage.CodeWaiting {
			return errors.New("archive already processing")
		}
		file := ctx.Params("file", "")
		if file == "" {
			return errors.New("file is empty")
		}
		file, err := url.QueryUnescape(file)
		if err != nil {
			return errors.New("cannot decode file")
		}
		return store.Delete(file)
	})
	app.Get("/zip/:id", func(ctx *fiber.Ctx) error {
		store := storageFactory.Session(ctx.Params("id", ""))
		if store.StatusCode() == storage.CodeWaiting {
			go store.Zip()
		}
		err := ctx.JSON(map[string]any{
			"code":     store.StatusCode(),
			"progress": store.StatusProgress(),
		})
		if err != nil {
			panic(err)
		}
		return nil
	})
	app.Get("/download/:id", func(ctx *fiber.Ctx) error {
		store := storageFactory.Session(ctx.Params("id", ""))
		if store.StatusCode() != storage.CodeFinished {
			return errors.New("archive haven't done")
		}
		defer store.Reset()
		return ctx.SendFile(store.ZipPath())
	})
	err := app.Listen("0.0.0.0:8080")
	if err != nil {
		panic(err)
	}
}

func readForm(ctx *fiber.Ctx) (*multipart.Form, error) {
	_, params, err := mime.ParseMediaType(ctx.GetReqHeaders()["Content-Type"])
	if err != nil {
		return nil, err
	}
	bodyReader := bytes.NewReader(ctx.Body())
	multipartReader := multipart.NewReader(bodyReader, params["boundary"])
	return multipartReader.ReadForm(1024)
}

func extractFile(form *multipart.Form) (*multipart.FileHeader, error) {
	files, ok := form.File["file"]
	if !ok {
		return nil, errors.New("file field not present")
	}
	if len(files) != 1 {
		return nil, errors.New("only one file per request")
	}
	return files[0], nil
}
