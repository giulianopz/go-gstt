package opts

import "strconv"

const (
	DefaultUserAgent  = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36"
	DefaultSampleRate = 16000
)

type Options struct {
	Verbose    bool
	FilePath   string
	ApiKey     string
	Output     string
	Language   string
	Continuous bool
	Interim    bool
	MaxAlts    string
	PFilter    string
	UserAgent  string
	Pair       string
	SampleRate int
}

type Option func(*Options)

func Apply(options ...Option) *Options {
	opts := &Options{}
	for _, o := range options {
		o(opts)
	}
	return opts
}

// Verbose output
func Verbose(enabled bool) Option {
	return func(o *Options) {
		o.Verbose = enabled
	}
}

// Path of audio file to trascript
func FilePath(path string) Option {
	return func(o *Options) {
		o.FilePath = path
	}
}

// API key built into Chrome
func ApiKey(key string) Option {
	return func(o *Options) {
		o.ApiKey = key
	}
}

type OutFmt int

const (
	Text OutFmt = iota
	Binary
)

// Output format
func Output(fmt OutFmt) Option {
	return func(o *Options) {
		if fmt == Text {
			o.Output = "json"
		} else {
			o.Output = "pb"
		}
	}
}

// Language of the recording transcription.
// Use the standard codes for your language, i.e. 'en-US' for English-US, 'ru' for Russian, etc. please, see https://en.wikipedia.org/wiki/IETF_language_tag
func Language(lang string) Option {
	return func(o *Options) {
		o.Language = lang
	}
}

// Keep the stream open and transcoding as long as there is no silence
func Continuous(enabled bool) Option {
	return func(o *Options) {
		o.Continuous = enabled
	}
}

// Send back results before its finished, so you get a live stream of possible transcriptions as it processes the audio
func Interim(enabled bool) Option {
	return func(o *Options) {
		o.Interim = enabled
	}
}

// How many possible transcriptions do you want
func MaxAlts(max int) Option {
	return func(o *Options) {
		o.MaxAlts = strconv.Itoa(max)
	}
}

// Profanity filter ('0'=off, '1'=medium, '2'=strict)
func ProfanityFilter(level int) Option {
	return func(o *Options) {
		o.PFilter = strconv.Itoa(level)
	}
}

// User-agent for spoofing (default 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36')
func UserAgent(agent string) Option {
	return func(o *Options) {
		o.UserAgent = agent
	}
}

// SampleRate is the sampling rate, i.e. the number of samples per second taken from a continuous signal to make a discrete or digital (default 16000)
func SampleRate(rate int) Option {
	return func(o *Options) {
		o.SampleRate = rate
	}
}

func GetOrDefault[V any](val, defaultVal V) V {
	switch v := any(val).(type) {
	case string:
		if v == "" {
			return defaultVal
		}
	case int:
		if v == 0 {
			return defaultVal
		}
	}
	return val
}
