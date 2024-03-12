package utils

import (
	"encoding/gob"
	"os"
)

// WriteGob encodes and stores data to a file.
// It takes the data to be stored and the file path as input.
// It returns an error if any.
func WriteGob(data interface{}, filePath string) error {

	file, err := os.Create(filePath)

	if err != nil {
		return err
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	if err = encoder.Encode(data); err != nil {
		return err
	}
	return nil
}

// ReadGob decodes and loads data from a file into the target variable.
// It takes the file path and a pointer to the target variable as input.
// It returns an error if any.
func ReadGob(filePath string, target interface{}) error {
	readFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer readFile.Close()

	decoder := gob.NewDecoder(readFile)
	if err = decoder.Decode(target); err != nil {
		return err
	}

	return nil
}
