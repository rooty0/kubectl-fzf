package util

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// EncodeToFile writes the gzipped gob encoding of data to filePath. The data is
// written to a temporary file that is renamed over the destination once it is
// complete, so a reader never observes a half-written file: the completion
// binary loads these files while the server rewrites them in place.
func EncodeToFile(data interface{}, filePath string) error {
	logrus.Debugf("Writing encoded data in %s", filePath)

	var gobBuf bytes.Buffer
	enc := gob.NewEncoder(&gobBuf)
	err := enc.Encode(data)
	if err != nil {
		return errors.Wrap(err, "error encoding gob data")
	}

	writer, err := os.CreateTemp(filepath.Dir(filePath), filepath.Base(filePath)+".tmp")
	if err != nil {
		return errors.Wrap(err, "error creating target file")
	}
	tmpPath := writer.Name()
	defer func() {
		writer.Close()
		os.Remove(tmpPath)
	}()

	archiver := gzip.NewWriter(writer)
	archiver.Name = filePath

	if _, err = io.Copy(archiver, &gobBuf); err != nil {
		return errors.Wrap(err, "error writing compressed data")
	}
	if err = archiver.Close(); err != nil {
		return errors.Wrap(err, "error flushing gzip writer")
	}
	// os.CreateTemp uses 0600, the previous os.Create used 0666 before umask.
	if err = writer.Chmod(0644); err != nil {
		return errors.Wrap(err, "error setting target file permissions")
	}
	if err = writer.Close(); err != nil {
		return errors.Wrap(err, "error closing target file")
	}
	return errors.Wrap(os.Rename(tmpPath, filePath), "error renaming target file")
}

func LoadGobFromFile(e interface{}, filePath string) error {
	logrus.Debugf("Loading file %s", filePath)
	b, err := ioutil.ReadFile(filePath)
	if err != nil {
		return errors.Wrap(err, "error reading file")
	}
	return DecodeGob(e, b)
}

func DecodeGob(e interface{}, b []byte) error {
	bbuffer := bytes.NewBuffer(b)
	zr, err := gzip.NewReader(bbuffer)
	if err != nil {
		return errors.Wrap(err, "error creating new gzip reader")
	}
	dec := gob.NewDecoder(zr)
	err = dec.Decode(e)
	if err := zr.Close(); err != nil {
		return errors.Wrap(err, "error closing gzip reader")
	}
	return err
}
