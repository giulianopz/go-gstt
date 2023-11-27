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

- xhw = hardware information - again, not sure how to use it.
*/

const (
	serviceURL        = "https://www.google.com/speech-api/full-duplex/v1"
	defaultSampleRate = 16000
)

var (
	logger          *slog.Logger
	defautlLogLevel = slog.LevelInfo
	wg              sync.WaitGroup
	client          = &http.Client{Transport: &http3.RoundTripper{}}
)

var (
	verbose    bool
	filePath   string
	apiKey     string
	output     string
	language   string
	continuous bool
	interim    bool
	maxAlts    string
	pFilter    string
)

func main() {

	flag.BoolVar(&verbose, "v", false, "verbose")
	flag.StringVar(&filePath, "f", "", "path of audio file to trascript")
	flag.StringVar(&apiKey, "k", "", "api key built into chromium")
	flag.StringVar(&output, "o", "", "output")
	flag.StringVar(&language, "l", "null", "language")
	flag.BoolVar(&continuous, "c", false, "continuous")
	flag.BoolVar(&interim, "i", false, "interim")
	flag.StringVar(&maxAlts, "max-alts", "1", "max alternatives")
	flag.StringVar(&pFilter, "pfilter", "2", "pFilter")
	flag.Parse()

	if verbose {
		defautlLogLevel = slog.LevelDebug
	}
	logger = slog.New(newLevelHandler(
		defautlLogLevel,
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
	))

	if filePath == "" {

		bs := make([]byte, 1024)

		// POST HTTP streaming
		pr, pw := io.Pipe()
		go func() {
			defer pr.Close()
			defer pw.Close()

			process(defaultSampleRate, pr)
		}()

		for {
			n, err := os.Stdin.Read(bs)
			if n > 0 {
				logger.Debug("read from stdin", "bs", bs)

				_, err := pw.Write(bs)
				if err != nil {
					panic(err)
				}
			} else if err == io.EOF {
				logger.Info("done reading from stdin")
				break
			} else if err != nil {
				logger.Error("could not read from stdin", "err", err)
				os.Exit(1)
			}
		}
	} else {

		f, err := goflac.ParseFile(filePath)
		if err != nil {
			logger.Error("cannot parse file", "err", err)
			os.Exit(1)
		}
		data, err := f.GetStreamInfo()
		if err != nil {
			logger.Error("cannot get file info", "err", err)
			os.Exit(1)
		}
		logger.Info("done parsing file", "sample rate", data.SampleRate)

		process(data.SampleRate, bytes.NewBuffer(f.Marshal()))
	}
}

func process(sampleRate int, r io.Reader) {
	pair := generatePair()

	wg.Add(1)
	go func() {
		defer wg.Done()
		send(client, pair, sampleRate, r)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		recv(client, pair)
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

func upQueryParams(pair string) url.Values {
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

func downQueryParams(pair string) url.Values {
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

func recv(c *http.Client, pair string) {
	downURL := encode(mustParse(serviceURL+"/down"), downQueryParams(pair))
	req, err := http.NewRequest(http.MethodGet, downURL, nil)
	if err != nil {
		panic(err)
	}
	// spoofing
	req.Header.Add("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")

	rsp, err := c.Do(req)
	if err != nil {
		logger.Error("DOWN", "err", err)
		os.Exit(1)
	}
	logger.Info("DOWN", "rsp", rsp)
	defer rsp.Body.Close()

	// GET HTTP streaming
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
			logger.Error("cannot unmarshal json", "err", err, "body", bs.String())
		}

		for _, res := range speechRecogResp.Result {
			for _, alt := range res.Alternative {
				logger.Info("result", "confidence", alt.Confidence, "transcript", alt.Transcript)
			}
		}
	}

}

func send(c *http.Client, pair string, sampleRate int, r io.Reader) {

	upURL := encode(mustParse(serviceURL+"/up"), upQueryParams(pair))
	req, err := http.NewRequest(http.MethodPost, upURL, r)
	if err != nil {
		panic(err)
	}
	// TODO make user agent configurable
	// spoofing
	req.Header.Add("user-agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36")
	req.Header.Add("content-type", "audio/x-flac; rate="+strconv.Itoa(sampleRate))

	rsp, err := c.Do(req)
	if err != nil {
		logger.Error("UP", "err", err, "rsp", rsp)
		if rsp != nil {
			buff := &bytes.Buffer{}
			_, err = io.Copy(buff, rsp.Body)
			if err != nil {
				panic(err)
			}
			logger.Error("UP", "err", err, "body", buff.String())
		}
		return
	}
	defer rsp.Body.Close()
	logger.Debug("UP", "rsp", rsp)

	buff := &bytes.Buffer{}
	_, err = io.Copy(buff, rsp.Body)
	if err != nil {
		panic(err)
	}
	logger.Debug("UP", "body", buff.String())
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
