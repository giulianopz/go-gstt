package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"

	goflac "github.com/go-flac/go-flac"
	"github.com/quic-go/quic-go/http3"
)

/*
Query parameters:

- key = the API key for the service.

- pFilter = profanity filter (0=off, 1=medium, 2=strict)

- lang = The language of the recording /
transcription. Use the standard webcodes for
your language. I.e. en-US for English-US, ru for
Russian, etc. https://en.wikipedia.org/wiki/IETF_language_tag

- output = The output of the stream. Some
values include “pb” for binary, “json” for json
string.

- pair = required for using the full-duplex stream.
A random alphanumeric string at least 16
characters long used in both up and down
streams.

- maxAlternatives = How many possible
transcriptions do you want? 1 - X.
continuous = Used in full-duplex to keep the
stream open and transcoding as long as there is
no silence

- interim = tells chrome to send back results
before its finished, so you get a live stream of
possible transcriptions as it processes the audio.
For the one_shot api there are a few other
options

- lm = Grammars to use - not sure how to use
this, believe its used to specify the type of
audio, ie transcription, message, etc.
https://cloud.google.com/speech-to-text/docs/reference/rest/v1/RecognitionConfig#interactiontype

- xhw = hardware information - again, not sure
how to use it.
*/

const (
	downEndpoint = "https://www.google.com/speech-api/full-duplex/v1/down"
	upEndpoint   = "https://www.google.com/speech-api/full-duplex/v1/up"
)

func main() {

	//verbose := flag.Bool("v", false, "verbose")
	filePath := flag.String("f", "", "path of audio file to trascript")
	apiKey := flag.String("k", "", "api key built into chromium")
	output := flag.String("o", "", "output")
	language := flag.String("l", "null", "language")
	interim := flag.Bool("i", false, "interim")
	maxAlts := flag.String("max-alts", "1", "max alternatives")
	pFilter := flag.String("pfilter", "2", "pFilter")

	flag.Parse()

	hclient := &http.Client{
		Transport: &http3.RoundTripper{},
	}

	pair := generatePair()

	values := url.Values{}
	values.Add("app", "chromium")
	values.Add("app", "continuous")
	if interim != nil && *interim {
		values.Add("app", "interim")
	}
	values.Add("maxAlternatives", *maxAlts)
	values.Add("pFilter", *pFilter)
	values.Add("lang", *language)
	values.Add("key", *apiKey)
	values.Add("pair", pair)
	values.Add("output", *output)

	var wg sync.WaitGroup

	wg.Add(1)
	go func(addr string) {

		if filePath != nil && *filePath != "" {
			f, err := goflac.ParseFile(*filePath)
			if err != nil {
				log.Fatal(err)
			}
			data, err := f.GetStreamInfo()
			if err != nil {
				log.Fatal(err)
			}

			slog.Info("sample rate", slog.Attr{})

			send(hclient, addr, data.SampleRate, f.Marshal())
		} else {

			// https://raw.githubusercontent.com/GoogleCloudPlatform/golang-samples/afa8430cf3ba1094b823aa17c94d9effb78b79d4/speech/livecaption/livecaption.go
			// gst-launch-1.0 -v pulsesrc ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout | go run .
			// FIX read from mic input until silence is detected
			buf := make([]byte, 1024)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					log.Printf("Pipe mic input to WebSpeechAPI")
					send(hclient, addr, 16000, buf)
				} else if err == io.EOF {
					log.Printf("Done reading from stdin")
					break
				} else if err != nil {
					log.Printf("Could not read from stdin: %v", err)
					continue
				}
			}

		}
		wg.Done()
	}(upEndpoint + "?" + values.Encode())

	values = url.Values{}
	values.Add("key", *apiKey)
	values.Add("pair", pair)
	values.Add("output", *output)

	wg.Add(1)
	go func(addr string) {
		recv(hclient, addr)
		wg.Done()
	}(downEndpoint + "?" + values.Encode())

	wg.Wait()
}

func generatePair() string {
	alphabet := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ret := ""
	for i := 0; i < 16; i++ {
		ret += string(alphabet[rand.Intn(len(alphabet)-1)+1])
	}
	return ret
}

func recv(hclient *http.Client, addr string) {
	rsp, err := hclient.Get(addr)
	if err != nil {
		log.Fatal(err)
	}
	slog.Info("DOWN", "response", rsp)
	defer rsp.Body.Close()

	dec := json.NewDecoder(rsp.Body)
	for {
		speechRecogResp := &response{}
		err = dec.Decode(speechRecogResp)
		if err == io.EOF {
			break
		}
		if err != nil {
			bs := &bytes.Buffer{}
			_, err = io.Copy(bs, rsp.Body)
			if err != nil {
				panic(err)
			}
			log.Fatal("cannot unmarshal json", err, bs.String())
		}

		for _, res := range speechRecogResp.Result {
			for _, alt := range res.Alternative {
				slog.Info("got", slog.Attr{Key: "confidence", Value: slog.AnyValue(alt.Confidence)}, "transcript", alt.Transcript)
			}
		}
	}
}

func send(hclient *http.Client, addr string, sampleRate int, bs []byte) {
	rsp, err := hclient.Post(addr, "audio/x-flac; rate="+strconv.Itoa(sampleRate), bytes.NewBuffer(bs))
	if err != nil {
		log.Fatal(err)
	}
	slog.Info("UP", "response", rsp)
	defer rsp.Body.Close()
}

type response struct {
	Result []struct {
		Alternative []struct {
			Transcript string  `json:"transcript,omitempty"`
			Confidence float64 `json:"confidence,omitempty"`
		} `json:"alternative,omitempty"`
		Final bool `json:"final,omitempty"`
	} `json:"result,omitempty"`
	ResultIndex int `json:"result_index,omitempty"`
}
