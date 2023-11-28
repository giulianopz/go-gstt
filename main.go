package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	goflac "github.com/go-flac/go-flac"
	"github.com/quic-go/quic-go/http3"
)

const (
	serviceURL        = "https://www.google.com/speech-api/full-duplex/v1"
	defaultUserAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"
	defaultSampleRate = 16000
)

var (
	logger          *slog.Logger
	defautlLogLevel = slog.LevelWarn
	wg              sync.WaitGroup
	client          = &http.Client{Transport: &http3.RoundTripper{}}
)

const usage = `Usage:
    gstt [OPTION]... -key $KEY -output [pb|json]
    gstt [OPTION]... -key $KEY --interim -continuous -output [pb|json]

Options:
	--verbose
	--file, path of audio file to trascript
	--key, api key built into chromium
	--output, transcriptions output format ('pb' for binary or 'json' for JSON messages)
	--language, language of the recording transcription, use the standard webcodes for your language, i.e. 'en-US' for English-US, 'ru' for Russian, etc. please, see https://en.wikipedia.org/wiki/IETF_language_tag
	--continuous, to keep the stream open and transcoding as long as there is no silence
	--interim, to send back results before its finished, so you get a live stream of possible transcriptions as it processes the audio
	--max-alts, how many possible transcriptions do you want
	--pfilter, profanity filter ('0'=off, '1'=medium, '2'=strict)
	--user-agent, user-agent for spoofing
`

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
	userAgent  string
)

func main() {

	flag.BoolVar(&verbose, "verbose", false, "verbose")
	flag.StringVar(&filePath, "file", "", "path of audio file to trascript")
	flag.StringVar(&apiKey, "key", "", "api key built into chromium")
	flag.StringVar(&output, "output", "", "output format ('pb' for binary or 'json' for JSON messages)")
	flag.StringVar(&language, "language", "null", "language of the recording transcription, use the standard webcodes for your language, i.e. 'en-US' for English-US, 'ru' for Russian, etc. please, see https://en.wikipedia.org/wiki/IETF_language_tag")
	flag.BoolVar(&continuous, "continuous", false, "to keep the stream open and transcoding as long as there is no silence")
	flag.BoolVar(&interim, "interim", false, "to send back results before its finished, so you get a live stream of possible transcriptions as it processes the audio")
	flag.StringVar(&maxAlts, "max-alts", "1", "how many possible transcriptions do you want")
	flag.StringVar(&pFilter, "pfilter", "2", "profanity filter ('0'=off, '1'=medium, '2'=strict)")
	flag.StringVar(&userAgent, "user-agent", defaultUserAgent, "user-agent for spoofing")
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	if verbose {
		defautlLogLevel = slog.LevelDebug
	}

	logger = slog.New(newLevelHandler(
		defautlLogLevel,
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}),
	))

	if apiKey == "" {
		logger.Error("'key' flag is mandatory")
		os.Exit(1)
	}

	if filePath != "" {

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

	} else {

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
	req.Header.Add("user-agent", defaultUserAgent)

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
				fmt.Fprintf(os.Stdout, "%s\n", strings.TrimSpace(alt.Transcript))
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

	req.Header.Add("user-agent", defaultUserAgent)
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
