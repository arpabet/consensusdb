/*
 * Copyright (c) 2025 Karagatan LLC.
 * SPDX-License-Identifier: BUSL-1.1
 */

package cdb

import (
	"time"
	"strconv"
	"golang.org/x/xerrors"
	"os"
	"io/ioutil"
	"strings"
	"unicode"
)

const (
	Day                  = time.Hour * 24
	Month                = Day * 31
	Year                 = Day * 365
)

func MaxInt(x, y int) int {
	if x >= y {
		return x
	} else {
		return y
	}
}

func CopyOf(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

func ToPrintable(data []byte) interface{} {
	str := string(data)
	for _, r := range str {
		if r > unicode.MaxASCII || !unicode.IsPrint(r) {
			return data
		}
	}
	return str
}

func ParseTtlExpr(ttlExpr string) (ttl time.Duration, err error) {

	len := len(ttlExpr)

	if len == 0 {
		return 0, nil
	}

	num, err := strconv.ParseInt(ttlExpr[:len-1], 10, 64)

	if err != nil {
		return 0, err
	}

	term := ttlExpr[len-1]

	switch term {

	case 'Y':
		return time.Duration(num) * Year, nil;

	case 'M':
		return time.Duration(num) * Month, nil;

	case 'D':
		return time.Duration(num) * Day, nil;

	case 'h':
		return time.Duration(num) * time.Hour, nil;

	case 'm':
		return time.Duration(num) * time.Minute, nil;

	case 's':
		return time.Duration(num) * time.Second, nil;

	}

	return 0, xerrors.New("unknown term: " + ttlExpr)

}


func CreateDirsIfNotExist(dirs ...string) error {

	for _, dir := range dirs {

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			err = os.Mkdir(dir, 0755)
			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CountFilesInDir(dir, ext string) int {

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return 0
	}

	cnt := 0

	for _, file := range files {

		if !file.IsDir() && strings.HasSuffix(file.Name(), ext)  {
			cnt = cnt + 1
		}

	}

	return cnt

}

func ReadAllKeys(blockC <-chan Block) []Key {

	list := make([]Key, 0, 100)

	for {
		block, ok := <- blockC

		if !ok {
			break
		}

		for _, rec := range block {
			list = append(list, rec.Key())
		}

	}

	return list

}


func ReadAll(blockC <-chan Block) []Record {

	list := make([]Record, 0, 100)

	for {
		block, ok := <- blockC

		if !ok {
			break
		}

		for _, rec := range block {
			list = append(list, rec)
		}

	}

	return list

}