# gsst

A Go client to call the Google Speech API for free.

The Google Speech API (full duplex version) are meant to offer a speech recognition service via the [Web Speech API](https://developer.mozilla.org/en-US/docs/Web/API/Web_Speech_API/Using_the_Web_Speech_API) on the [Google Chrome](https://source.chromium.org/chromium/chromium/src/+/main:content/browser/speech/speech_recognition_engine.cc) browser. They are different from the [Google Cloud Speech-to-Text API](https://cloud.google.com/speech-to-text/v2/docs). 

The API can be called using the API key built into Chrome. To find your key, go to the [demo](https://www.google.com/intl/en/chrome/demos/speech.html) page for the Web Speech API on Chrome and capture the network traffic with the Chrome [net export](chrome://net-export/) tool. Then, inspect the logs with the [netlog viewer](https://netlog-viewer.appspot.com/#import) tool.

> Disclaimer: The Google Speech API are not meant for commercial use. Also, please consider asking for a [key](https://www.chromium.org/developers/how-tos/api-keys/) reserved to developers.

### Usage

```bash
git clone https://github.com/giulianopz/go-gsst
cd go-gsst
go build -o gsst .
gsst -h

# trascribe audio from a single FLAC file
gsst --interim --continuous --key $KEY --output json --file $FILE
# trascribe audio from microphone input (recorded with sox, removing silence)
rec -c 1 --encoding signed-integer --bits 16 --rate 16000 -t flac - silence 1 0.1 1% -1 0.5 1% | gsst --interim --continuous --key $KEY --output json
```

### Credits

As far as I know, this API has been going around since a long time, although its endpoint was updated and finally moved to the current one.  

[Mike Pultz](https://mikepultz.com/2011/03/accessing-google-speech-api-chrome-11/) possibly was the first to discover it.

[Travis Payton](http://blog.travispayton.com/wp-content/uploads/2014/03/Google-Speech-API.pdf) published a detailed report on the subject.


