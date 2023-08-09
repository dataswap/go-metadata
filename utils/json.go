package utils

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

func PathExists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsExist(err) {
		return true
	}

	return false
}

func ReadJson(filePath string, val interface{}) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return err
	}

	if err = json.Unmarshal(data, val); err != nil {
		return err
	}

	return nil
}

func WriteJson(filePath string, indent string, val interface{}) error {
	var err error
	var data []byte
	if indent == "" {
		data, err = json.Marshal(val)
	} else {
		data, err = json.MarshalIndent(val, "", indent)
	}
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filePath, data, 0644)
}
