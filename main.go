package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
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
	serviceURL = "https://www.google.com/speech-api/full-duplex/v1"
)

func main() {

	var (
		wg     sync.WaitGroup
		pair   = generatePair()
		client = &http.Client{Transport: &http3.RoundTripper{}}
	)

	//verbose := flag.Bool("v", false, "verbose")
	filePath := flag.String("f", "", "path of audio file to trascript")
	apiKey := flag.String("k", "", "api key built into chromium")
	output := flag.String("o", "", "output")
	language := flag.String("l", "null", "language")
	continuous := flag.Bool("c", false, "continuous")
	interim := flag.Bool("i", false, "interim")
	maxAlts := flag.String("max-alts", "1", "max alternatives")
	pFilter := flag.String("pfilter", "2", "pFilter")
	flag.Parse()

	upURL := encode(mustParse(serviceURL+"/up"), upQueryParams(*continuous, *interim, *maxAlts, *pFilter, *language, *apiKey, pair, *output))
	downURL := encode(mustParse(serviceURL+"/down"), downQueryParams(*apiKey, pair, *output))

	wg.Add(1)
	go func() {
		defer wg.Done()

		if filePath != nil && *filePath != "" {
			f, err := goflac.ParseFile(*filePath)
			if err != nil {
				slog.Error("cannot parse file", "err", err)
				os.Exit(1)
			}
			data, err := f.GetStreamInfo()
			if err != nil {
				slog.Error("cannot get file info", "err", err)
				os.Exit(1)
			}

			slog.Info("UP", "sample rate", data.SampleRate)

			send(client, upURL, data.SampleRate, f.Marshal())
		} else {

			// https://raw.githubusercontent.com/GoogleCloudPlatform/golang-samples/afa8430cf3ba1094b823aa17c94d9effb78b79d4/speech/livecaption/livecaption.go
			// gst-launch-1.0 -v pulsesrc ! audioconvert ! audioresample ! audio/x-raw,channels=1,rate=16000 ! filesink location=/dev/stdout | go run .
			// FIX read from mic input until silence is detected
			buf := make([]byte, 1024)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					send(client, upURL, 16000, buf)
				} else if err == io.EOF {
					slog.Info("Done reading from stdin")
					break
				} else if err != nil {
					slog.Error("Could not read from stdin", "err", err)
					continue
				}
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		recv(client, downURL)
	}()

	wg.Wait()
}

func mustParse(s string) *url.URL {
	ret, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return ret
}

func upQueryParams(continuous, interim bool, maxAlts, pFilter, language, apiKey, pair, output string) url.Values {
	values := url.Values{}
	values.Add("app", "chromium")
	if interim {
		values.Add("interim", "")
	}
	if continuous {
		values.Add("continuous", "")
	}
	values.Add("maxAlternatives", maxAlts)
	values.Add("pFilter", pFilter)
	values.Add("lang", language)
	values.Add("key", apiKey)
	values.Add("pair", pair)
	values.Add("output", output)
	return values
}

func downQueryParams(apiKey, pair, output string) url.Values {
	values := url.Values{}
	values.Add("key", apiKey)
	values.Add("pair", pair)
	values.Add("output", output)
	return values
}

func encode(base *url.URL, queryParams url.Values) string {
	base.RawQuery = queryParams.Encode()
	return base.String()
}

func generatePair() string {
	alphabet := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ret := ""
	for i := 0; i < 16; i++ {
		ret += string(alphabet[rand.Intn(len(alphabet)-1)+1])
	}
	return ret
}

func recv(c *http.Client, addr string) {
	req, err := http.NewRequest(http.MethodGet, addr, nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")

	rsp, err := c.Do(req)
	if err != nil {
		slog.Error("DOWN", "err", err)
		os.Exit(1)
	}
	slog.Info("DOWN", "rsp", rsp)
	defer rsp.Body.Close()

	/* 	bs := &bytes.Buffer{}
	   	_, err = io.Copy(bs, rsp.Body)
	   	if err != nil {
	   		panic(err)
	   	}
	   	slog.Info("DOWN", "result", bs) */

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
			slog.Error("cannot unmarshal json", "err", err, "bs", bs.String())
			os.Exit(1)
		}

		for _, res := range speechRecogResp.Result {
			for _, alt := range res.Alternative {
				slog.Info("result", slog.Attr{Key: "confidence", Value: slog.AnyValue(alt.Confidence)}, "transcript", alt.Transcript)
			}
		}
	}

}

func send(c *http.Client, addr string, sampleRate int, bs []byte) {

	req, err := http.NewRequest(http.MethodPost, addr, bytes.NewBuffer(bs))
	if err != nil {
		panic(err)
	}
	req.Header.Add("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Add("content-type", "audio/x-flac; rate="+strconv.Itoa(sampleRate))

	slog.Info("UP", "addr", addr)

	rsp, err := c.Do(req)
	if err != nil {
		slog.Error("UP", "err", err, "rsp", rsp)
		buff := &bytes.Buffer{}
		_, err = io.Copy(buff, rsp.Body)
		if err != nil {
			panic(err)
		}
		slog.Error("UP", "err", err, "bs", buff.String())
		os.Exit(1)
	}
	defer rsp.Body.Close()
	slog.Info("UP", "rsp", rsp)

	buff := &bytes.Buffer{}
	_, err = io.Copy(buff, rsp.Body)
	if err != nil {
		panic(err)
	}
	slog.Error("UP", "err", err, "bs", buff.String())
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
