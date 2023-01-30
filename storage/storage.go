package storage

import (
	"archive/zip"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path"
)

const (
	CodeWaiting = iota
	CodeRunning
	CodeFinished
)

type Partial struct {
	statuses map[string]*Storage
	dir      string
}

type Storage struct {
	dir, session, zip string
	status            Status
}

type Status struct {
	Code     int
	Progress int
}

func New(dir string) *Partial {
	err := os.MkdirAll(dir, os.ModeDir)
	os.Chmod(dir, 0777)
	os.Chown(dir, os.Getuid(), os.Getgid())
	if err != nil {
		panic(err)
	}
	return &Partial{map[string]*Storage{}, dir}
}

func NewTemp(dir string) *Partial {
	return New(path.Join(os.TempDir(), dir))
}

func (p Partial) Session(session string) *Storage {
	storage, ok := p.statuses[session]
	if !ok {
		pt := path.Join(p.dir, session)
		err := os.MkdirAll(pt, os.ModeDir)
		os.Chmod(pt, 0777)
		if err != nil {
			panic(err)
		}
		storage = &Storage{
			p.dir,
			session,
			"",
			Status{
				CodeWaiting,
				0,
			},
		}
		p.statuses[session] = storage
	}
	return storage
}

func (s *Storage) StatusCode() int {
	return s.status.Code
}

func (s *Storage) StatusProgress() int {
	return s.status.Progress
}

func (s *Storage) ZipPath() string {
	return s.zip
}

func (s *Storage) Create(file *multipart.FileHeader) string {
	createdFile, err := os.Create(path.Join(s.dir, s.session, file.Filename))
	os.Chmod(createdFile.Name(), 0777)
	defer func() {
		err = createdFile.Close()
		if err != nil {
			panic(err)
		}
	}()
	if err != nil {
		panic(err)
	}
	open, err := file.Open()
	if err != nil {
		panic(err)
	}
	copyToFile(open, file.Size, createdFile)
	err = open.Close()
	if err != nil {
		panic(err)
	}
	return file.Filename
}

func (s *Storage) Delete(filename string) error {
	p := path.Join(s.dir, s.session, filename)
	err := os.Remove(p)
	if err != nil {
		_, ok := err.(*os.PathError)
		if !ok {
			panic(err)
		}
		return errors.New("file does not exist")
	}
	return nil
}

func (s *Storage) Reset() {
	*s = Storage{
		dir:     s.dir,
		session: s.session,
		zip:     "",
		status: Status{
			Code:     CodeWaiting,
			Progress: 0,
		},
	}
}

func copyToFile(src multipart.File, srcLength int64, dst *os.File) {
	chunkSize := int64(1024)
	buffer := make([]byte, chunkSize)
	for i := int64(0); i*chunkSize < srcLength; i++ {
		offset := i * chunkSize
		_, err := src.ReadAt(buffer, offset)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			panic(err)
		}
		_, err = dst.WriteAt(buffer, offset)
		if err != nil {
			panic(err)
		}
	}
}

func (s *Storage) Zip() {
	s.status.Code = CodeRunning
	zipName := path.Join(s.dir, s.session+"-result.zip")
	create, err := os.Create(zipName)
	os.Chmod(create.Name(), 0777)
	if err != nil {
		panic(err)
	}
	zipFile := zip.NewWriter(create)
	s.writeRecursive(zipFile)
	err = zipFile.Close()
	if err != nil {
		panic(err)
	}
	err = create.Close()
	if err != nil {
		panic(err)
	}
	s.status.Code = CodeFinished
	s.zip = zipName
}

func (s *Storage) writeRecursive(writer *zip.Writer) {
	entries, err := os.ReadDir(path.Join(s.dir, s.session))
	if err != nil {
		panic(err)
	}
	total := len(entries)
	for done, entry := range entries {
		if entry.IsDir() {
			panic("invalid state: inner directory detected")
		} else {
			header := &zip.FileHeader{}
			header.Name = path.Join(entry.Name())
			withHeader, err := writer.CreateHeader(header)
			if err != nil {
				panic(err)
			}
			file, err := os.ReadFile(path.Join(s.dir, s.session, entry.Name()))
			if err != nil {
				panic(err)
			}
			_, err = withHeader.Write(file)
			if err != nil {
				panic(err)
			}
			s.status.Progress = (done + 1) / total * 100
		}
	}
}
