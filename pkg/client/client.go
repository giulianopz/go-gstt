package client

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/giulianopz/go-gstt/pkg/logger"
	"github.com/giulianopz/go-gstt/pkg/opts"
	"github.com/giulianopz/go-gstt/pkg/transcription"
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

func (c *client) send(body io.Reader, options *opts.Options) {
	req, err := http.NewRequest(http.MethodPost, getUrl("up", options), body)
	if err != nil {
		panic(err)
	}

	req.Header.Add("user-agent", opts.GetOrDefault(options.UserAgent, opts.DefaultUserAgent))
	req.Header.Add("content-type", "audio/x-flac; rate="+strconv.Itoa(
		opts.GetOrDefault(options.SampleRate, opts.DefaultSampleRate)),
	)

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

func (c *client) recv(out chan<- *transcription.Response, options *opts.Options) {
	req, err := http.NewRequest(http.MethodGet, getUrl("down", options), nil)
	if err != nil {
		panic(err)
	}

	req.Header.Add("user-agent", opts.GetOrDefault(options.UserAgent, opts.DefaultUserAgent))

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
		speechRecogResp := &transcription.Response{}

		err = dec.Decode(speechRecogResp)
		if err == io.EOF {
			close(out)
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

		out <- speechRecogResp
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

const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"

var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))

func generatePair() string {
	bs := make([]byte, 0)
	for i := 0; i < 16; i++ {
		bs = append(bs, charset[seededRand.Intn(len(charset))])
	}
	return string(bs)
}

// Transcribe sends an audio input to Google Speesch API printing to stdout its trascripts.
// Audio must be in FLAC codec/format and can be passed via any io.Reader valid implementation.
// If audio is coming from microphone input, its sample rate must be 16000.
// If a file, the sample rate must match the one declared in the file header.
// Result are received asynchronously via a channel.
// The options control the way audio is transcribed.
func (c *client) Transcribe(in io.Reader, out chan<- *transcription.Response, options *opts.Options) {

	options.Pair = generatePair()

	go func() {
		c.send(in, options)
	}()

	go func() {
		c.recv(out, options)
	}()
}
