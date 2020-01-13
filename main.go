package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strconv"

	"github.com/1set/gut/yhash"
	"github.com/1set/gut/yrand"
)

const (
	defaultExperimentTimes = 10000
	minBatchSize = 10000
)

func Float2Bytes(feature []float32) (data []byte, err error) {
	var feaBuf bytes.Buffer
	err = binary.Write(&feaBuf, binary.LittleEndian, feature)
	data = feaBuf.Bytes()
	return
}

func GetRandomFeatureBytes() (bytes []byte, err error) {
	const floatCount = 256

	numbers := make([]float32, 0, floatCount)
	for idx := 0; idx < floatCount; idx++ {
		var num float32
		if num, err = yrand.Float32(); err != nil {
			return
		}
		numbers = append(numbers, num)
	}

	bytes, err = Float2Bytes(numbers)
	return
}

func GetRandomFeatureHash() (hash string, err error) {
	var bytes []byte
	bytes, err = GetRandomFeatureBytes()

	if hash, err = yhash.BytesMD5(bytes); err == nil {
		if len(hash) >= 18 {
			hash = hash[:18]
		} else {
			err = fmt.Errorf("incomplete hash: %q, length: %d", hash, len(hash))
		}
	}

	return
}

func main() {
	fmt.Println(`usage: md5con [times]`)

	times := defaultExperimentTimes
	if len(os.Args) >= 2 {
		rawPort := os.Args[1]
		if num, err := strconv.Atoi(rawPort); err != nil {
			fmt.Printf("%q is not a times number: %v\n", rawPort, err)
			os.Exit(1)
		} else {
			times = num
		}
	}
	fmt.Println("experiment times:", times)

	batchSize := times / 1000
	if batchSize < minBatchSize {
		batchSize = minBatchSize
	}

	hashMap := map[string]bool{}

	for idx := 1; idx <= times; idx++ {
		hash, err := GetRandomFeatureHash()
		if err != nil {
			fmt.Printf("index #%d - got error: %v\n", idx, err)
			continue
		}

		if _, ok := hashMap[hash]; ok {
			fmt.Printf("index #%d - got conflict: %s\n", idx, hash)
			os.Exit(2)
		}

		hashMap[hash] = true
		if idx % minBatchSize == 0 {
			fmt.Printf("done: %.2f%% (%d) - %s\n", float64(idx) / float64(times) * 100, idx, hash)
		}
	}

	fmt.Println("all done for", times)
	os.Exit(0)
}
