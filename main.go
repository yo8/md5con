package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/1set/gut/yhash"
	"github.com/1set/gut/yrand"
)

const (
	numOfWorkers           = 32
	numOfRandSource        = 8
	defaultExperimentTimes = 2000000
	minBatchSize           = 200000
	gcBatchSize            = 1000000
	bufferChanSize         = 100000
	seedChanSize           = numOfWorkers * numOfRandSource * 256
	refreshRandSourcePer   = 10000
	saveFullFeature        = false
)

type Feature struct {
	Hash    string
	Content string
}

var (
	hashMap     = map[string]string{}
	seedChan    = make(chan int64, seedChanSize)
	featureChan = make(chan *Feature, bufferChanSize)
	errorChan   chan error
)

func Float2Bytes(feature []float32) (data []byte, err error) {
	var feaBuf bytes.Buffer
	err = binary.Write(&feaBuf, binary.LittleEndian, feature)
	data = feaBuf.Bytes()
	return
}

func GetRandomFeatureBytes(sources []*rand.Rand) (bytes []byte, err error) {
	const floatCount = 256

	numOfSource := len(sources)
	if numOfSource == 0 {
		err = fmt.Errorf("got no random source")
		return
	}

	source := sources[0]
	numbers := make([]float32, 0, floatCount)
	for idx := 0; idx < floatCount; idx++ {
		numbers = append(numbers, source.Float32())
		if idx%16 == 0 {
			source = sources[source.Int()%numOfSource]
		}
	}

	bytes, err = Float2Bytes(numbers)
	return
}

func GetRandomFeature(sources []*rand.Rand) (fea *Feature, err error) {
	var bytes []byte
	if bytes, err = GetRandomFeatureBytes(sources); err != nil {
		return
	}

	var hash string
	if hash, err = yhash.BytesMD5(bytes); err == nil {
		if len(hash) >= 18 {
			hash = hash[:18]
		} else {
			err = fmt.Errorf("incomplete hash: %q, length: %d", hash, len(hash))
		}
	}

	fea = &Feature{
		Hash:    hash,
		Content: "",
	}
	if saveFullFeature {
		fea.Content = base64.StdEncoding.EncodeToString(bytes)
	}

	return
}

func StartRandomSourceSeedGenerator() {
	fmt.Println("seeder starts")
	for {
		if num, err := yrand.Int64Range(-9223372036854775808, 9223372036854775800); err == nil {
			seedChan <- num
		} else {
			errorChan <- err
			break
		}
	}
	fmt.Println("seeder ends")
}

func StartRandomFeatureHashGenerator(id int) {
	var (
		feature *Feature
		err     error
	)

	fmt.Printf("generator #%d starts\n", id)

	count := 0
	sources := make([]*rand.Rand, 0, numOfRandSource)
	for {
		if count == 0 {
			for idx := 0; idx < numOfRandSource; idx++ {
				sources = append(sources, rand.New(rand.NewSource(<-seedChan)))
			}
		}
		count++

		if feature, err = GetRandomFeature(sources); err == nil {
			featureChan <- feature
		} else {
			errorChan <- err
			break
		}

		if count >= refreshRandSourcePer {
			count = 0
		}
	}
	fmt.Printf("generator #%d exits\n", id)
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

	go StartRandomSourceSeedGenerator()
	for idx := 1; idx <= numOfWorkers; idx++ {
		go StartRandomFeatureHashGenerator(idx)
	}

	var (
		hash *Feature
		err  error
	)

	startTime := time.Now()
	for idx := 1; idx <= times; idx++ {
		select {
		case hash = <-featureChan:
			if hash == nil {
				fmt.Printf("index #%d - got null feature: %v\n", idx, hash)
				continue
			}
		case err = <-errorChan:
			if err != nil {
				fmt.Printf("index #%d - got error: %v\n", idx, err)
				continue
			}
		}

		if content, ok := hashMap[hash.Hash]; ok {
			same := content == hash.Content
			fmt.Printf("index #%d - got conflict: %s, saved feature: %v, exact same: %v\n", idx, hash.Hash, saveFullFeature, same)
			if saveFullFeature && !same {
				fmt.Println("1st:", content)
				fmt.Println("2nd:", hash.Content)
			}
			os.Exit(2)
		}

		hashMap[hash.Hash] = hash.Content

		if idx%gcBatchSize == 0 {
			var memBefore, memAfter runtime.MemStats
			runtime.ReadMemStats(&memBefore)
			// runtime.GC()
			debug.FreeOSMemory()
			runtime.ReadMemStats(&memAfter)

			// For info on each, see: https://golang.org/pkg/runtime/#MemStats
			fmt.Printf("done: %.2f%% (%d) - GC #%d, Sys: %.2f MiB -> %.2f MiB, Alloc: %.2f MiB -> %.2f MiB, Total: %.2f MiB -> %.2f MiB\n",
				float64(idx)/float64(times)*100, idx,
				memAfter.NumGC,
				float64(memBefore.Sys)/1024/1024, float64(memAfter.Sys)/1024/1024,
				float64(memBefore.Alloc)/1024/1024, float64(memAfter.Alloc)/1024/1024,
				float64(memBefore.TotalAlloc)/1024/1024, float64(memAfter.TotalAlloc)/1024/1024,
			)
		} else if idx%minBatchSize == 0 {
			fmt.Printf("done: %.2f%% (%d) - %s - %.2f/sec\n", float64(idx)/float64(times)*100, idx, hash.Hash, float64(minBatchSize)/(time.Since(startTime).Seconds()))
			startTime = time.Now()
		}
	}

	fmt.Println("all done for", times)
	os.Exit(0)
}
