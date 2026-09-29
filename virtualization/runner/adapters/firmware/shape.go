package firmware

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

func matchesBuild(varsPath, template string) error {
	haveFormat, haveSize, err := pflashShape(varsPath)
	if err != nil {
		return err
	}
	wantFormat, wantSize, err := pflashShape(template)
	if err != nil {
		return err
	}
	if haveFormat != wantFormat || haveSize != wantSize {
		return fmt.Errorf("UEFI variable store %s is %s/%d bytes but selected firmware needs %s/%d; explicit offline migration is required",
			varsPath, haveFormat, haveSize, wantFormat, wantSize)
	}
	return nil
}

func pflashShape(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", 0, err
	}
	if !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("%s: firmware image must be a regular file", path)
	}
	var header [32]byte
	read, err := io.ReadFull(file, header[:])
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", 0, err
	}
	if read == len(header) && string(header[:4]) == "QFI\xfb" {
		return "qcow2", int64(binary.BigEndian.Uint64(header[24:32])), nil
	}
	return "raw", info.Size(), nil
}
