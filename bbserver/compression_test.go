package bbserver_test

import (
	"testing"
	"bigbagger/bbserver"
	"bigbagger/proto/bbproto"
	"bytes"
	"fmt"
	"reflect"
)

func TestCompressions(t *testing.T) {

	input := make([]byte, 1000, 1000)

	for i := 0; i < 1000; i = i+1 {
		input[i] = byte(i)
	}

	for _, v := range bbserver.KnownCompressions {

		for _, level := range bbproto.CompressionLevel_value {
			CompressionTest(t, input, v, bbproto.CompressionLevel(level))
		}

	}


}


func CompressionTest(t *testing.T, input []byte, compression bbserver.ICompression, level bbproto.CompressionLevel) {

	output, err := compression.Compress(input, level)
	if err != nil {
		t.Fatal("fail to compress ", err)
	}

	fmt.Print("output.len=", len(output), " for = ", reflect.TypeOf(compression), " level=", level, "\n")

	actual, err := compression.Decompress(output)
	if err != nil {
		t.Fatal("fail to decompress ", err)
	}

	if !bytes.Equal(input, actual) {
		t.Fatal("actual not the same as input", err)
	}

}