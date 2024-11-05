package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/giulianopz/go-gstt/pkg/client"
	"github.com/giulianopz/go-gstt/pkg/logger"
	"github.com/giulianopz/go-gstt/pkg/opts"
	"github.com/giulianopz/go-gstt/pkg/transcription"
	goflac "github.com/go-flac/go-flac"
)

const usage = `Usage:
    gstt [OPTION]... --interim --continuous [--file FILE]

Options:
	--verbose
	--file, path of audio file to trascript
	--key, api key built into chromium
	--language, language of the recording transcription, use the standard webcodes for your language, i.e. 'en-US' for English-US, 'ru' for Russian, etc. please, see https://en.wikipedia.org/wiki/IETF_language_tag
	--continuous, keeps the stream open and transcoding as long as there is no silence
	--interim, sends back results before its finished, so you get a live stream of possible transcriptions as it processes the audio
	--max-alts, how many possible transcriptions do you want
	--pfilter, profanity filter ('0'=off, '1'=medium, '2'=strict)
	--user-agent, user-agent for spoofing
	--sample-rate, audio sampling rate
	--subtitle-mode, shows the transcriptions as if they were subtitles, while playing the media file, clearing the screen at each transcription
`

var (
	verbose      bool
	filePath     string
	apiKey       string
	language     string
	continuous   bool
	interim      bool
	maxAlts      string
	pFilter      string
	userAgent    string
	sampleRate   int
	subtitleMode bool
)

func main() {

	flag.BoolVar(&verbose, "verbose", false, "verbose")
	flag.StringVar(&filePath, "file", "", "path of audio file to trascript")
	flag.StringVar(&apiKey, "key", "", "API key to authenticates request (default is the one built into any Chrome installation)")
	flag.StringVar(&language, "language", "null", "language of the recording transcription, use the standard codes for your language, i.e. 'en-US' for English-US, 'ru' for Russian, etc. please, see https://en.wikipedia.org/wiki/IETF_language_tag")
	flag.BoolVar(&continuous, "continuous", false, "to keep the stream open and transcoding as long as there is no silence")
	flag.BoolVar(&interim, "interim", false, "to send back results before its finished, so you get a live stream of possible transcriptions as it processes the audio")
	flag.StringVar(&maxAlts, "max-alts", "1", "how many possible transcriptions do you want")
	flag.StringVar(&pFilter, "pfilter", "2", "profanity filter ('0'=off, '1'=medium, '2'=strict)")
	flag.StringVar(&userAgent, "user-agent", opts.DefaultUserAgent, "user-agent for spoofing (default 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36')")
	flag.IntVar(&sampleRate, "sample-rate", opts.DefaultSampleRate, "audio sampling rate")
	flag.BoolVar(&subtitleMode, "subtitle-mode", false, "shows the transcriptions as if they were subtitles, while playing the media file, clearing the screen at each transcription")
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	if verbose {
		logger.Level(slog.LevelDebug)
	}

	var (
		httpC   = client.New()
		options = fromFlags()
		out     = make(chan *transcription.Response)
	)

	if filePath != "" { // transcribe from file

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
		options.SampleRate = data.SampleRate
		logger.Info("done parsing file", "sample rate", data.SampleRate)

		go httpC.Transcribe(bytes.NewBuffer(f.Marshal()), out, options)
	} else { // transcribe from microphone input

		pr, pw := io.Pipe()
		defer pr.Close()
		defer pw.Close()

		go func() {

			bs := make([]byte, 1024)
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
					logger.Error("cannot not read from stdin", "err", err)
					os.Exit(1)
				}
			}
		}()

		go httpC.Transcribe(pr, out, options)
	}

	for resp := range out {
		for _, result := range resp.Result {
			if !result.Final {
				continue
			}
			for _, alt := range result.Alternative {
				logger.Debug("got transcription", slog.Float64("confidence", alt.Confidence), slog.String("transcript", alt.Transcript))
				transcript := strings.TrimSpace(alt.Transcript)
				fmt.Printf("%s", transcript)
				if subtitleMode {
					// Assumimg reading speed = 238 WPM (words per minute)
					// see https://thereadtime.com/
					time.Sleep(time.Duration(float64(len(strings.Fields(transcript)))*0.26) * time.Second)
					// clear the entire screen with ANSI escapes
					fmt.Print("\x1b[H\x1b[2J\x1b[3J\n")
				} else {
					fmt.Println()
				}
			}
		}
	}
}

func fromFlags() *opts.Options {

	options := make([]opts.Option, 0)

	if verbose {
		options = append(options, opts.Verbose(true))
	}
	if filePath != "" {
		options = append(options, opts.FilePath(filePath))
	}
	if apiKey != "" {
		options = append(options, opts.ApiKey(apiKey))
	}
	if language != "" {
		options = append(options, opts.Language(language))
	}
	if continuous {
		options = append(options, opts.Continuous(true))
	}
	if interim {
		options = append(options, opts.Interim(true))
	}
	if maxAlts != "" {
		num, err := strconv.Atoi(maxAlts)
		if err != nil {
			panic(err)
		}
		options = append(options, opts.MaxAlts(num))
	}
	if pFilter != "" {
		num, err := strconv.Atoi(pFilter)
		if err != nil {
			panic(err)
		}
		options = append(options, opts.ProfanityFilter(num))
	}

	options = append(options, opts.UserAgent(userAgent))
	options = append(options, opts.SampleRate(sampleRate))

	return opts.Apply(options...)
}
