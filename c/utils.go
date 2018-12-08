/*
 *
 * Copyright 2018-present Alexander Shvid and Contributors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package c

import (
	"time"
	"strconv"
	"github.com/pkg/errors"
	"github.com/consensusdb/consensusdb/cserver/cserverpb"
	"os"
	"io/ioutil"
	"strings"
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

	return 0, errors.New("unknown term: " + ttlExpr)

}


func IsUpdateOperation(operation *cserverpb.TxOperation) bool {

	switch operation.Operation.(type) {

	case *cserverpb.TxOperation_Get:
		return false

	case *cserverpb.TxOperation_Range:
		return false

	case *cserverpb.TxOperation_Touch:
		return true

	case *cserverpb.TxOperation_Put:
		return true

	case *cserverpb.TxOperation_Remove:
		return true

	}

	return false

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