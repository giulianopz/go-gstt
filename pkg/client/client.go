package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/giulianopz/go-gsst/pkg/logger"
	"github.com/giulianopz/go-gsst/pkg/opts"
	"github.com/giulianopz/go-gsst/pkg/str"
	"github.com/quic-go/quic-go/http3"
)

const serviceURL = "https://www.google.com/speech-api/full-duplex/v1"

type client struct {
	*http.Client
}

// New returns a client for the Google Speech API
func New() *client {
	return &client{
		Client: &http.Client{Transport: &http3.RoundTripper{}},
	}
}

func (c *client) send(sampleRate int, body io.Reader, options *opts.Options) {
	req, err := http.NewRequest(http.MethodPost, getUrl("up", options), body)
	if err != nil {
		panic(err)
	}

	req.Header.Add("user-agent", str.GetOrDefault(options.UserAgent, opts.DefaultUserAgent))
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

func (c *client) recv(options *opts.Options) {
	req, err := http.NewRequest(http.MethodGet, getUrl("down", options), nil)
	if err != nil {
		panic(err)
	}

	req.Header.Add("user-agent", str.GetOrDefault(options.UserAgent, opts.DefaultUserAgent))

	rsp, err := c.Do(req)
	if err != nil {
		logger.Error("DOWN", "err", err)
		os.Exit(1)
	}
	logger.Info("DOWN", "rsp", rsp)
	defer rsp.Body.Close()

	// stream GET response body
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

func getUrl(streamDirection string, options *opts.Options) string {
	u, err := url.Parse(serviceURL + "/" + streamDirection)
	if err != nil {
		panic(err)
	}

	var values = url.Values{}

	if options.ApiKey != "" {
		values.Add("key", options.ApiKey)
	} else {
		panic("missing api key")
	}
	if options.Pair != "" {
		values.Add("pair", options.Pair)
	}
	if options.Output != "" {
		values.Add("output", options.Output)
	}

	if streamDirection == "up" {
		values.Add("app", "chromium")
		if options.Interim {
			values.Add("interim", "")
		}
		if options.Continuous {
			values.Add("continuous", "")
		}
		if options.MaxAlts != "" {
			values.Add("maxAlternatives", options.MaxAlts)
		}
		if options.PFilter != "" {
			values.Add("pFilter", options.PFilter)
		}
		if options.Language != "" {
			values.Add("lang", options.Language)
		}
	}

	u.RawQuery = values.Encode()
	logger.Debug("got", "url", u.String())
	return u.String()
}

func generatePair() string {
	alphabet := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	ret := ""
	for i := 0; i < 16; i++ {
		ret += string(alphabet[rand.Intn(len(alphabet)-1)+1])
	}
	return ret
}

// Stream sends an audio input to Google Speesch API printing to stdout its trascripts.
// Audio must be in FLAC codec/format. If audio is coming from microphone input, its sample rate
// must be 16000. If a file, the sample rate must match the one declared in the file header.
// The options control the audio transcription.
func (c *client) Stream(audio io.Reader, sampleRate int, options *opts.Options) {

	wg := sync.WaitGroup{}

	options.Pair = generatePair()

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.send(sampleRate, audio, options)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		c.recv(options)
	}()

	wg.Wait()
}
