# FLAPI

FLAPI is a fully offline, containerized, GPU-ready, automatic-speech-recognition (ASR) websocket API built
on top of [Flashlight](https://github.com/facebookresearch/flashlight)'s
[ASR library](https://github.com/facebookresearch/flashlight/tree/master/flashlight/app/asr).

---

## Quick start

```sh
# prepare a persistent directory to store the model files.
# You'll need about 10GB, ideally on your fastest SSD for dev.
HOST_DATA="$HOME/flapi-data"

# define the port on which you want to expose the HTTP API
HOST_PORT=8080

mkdir -p "$HOST_DATA"
git clone https://github.com/cowdude/flapi
cp flapi/data/hello.wav "$HOST_DATA/"
flapi/download_models.sh "$HOST_DATA"
docker run \
    -v "$HOST_DATA:/data" \
    -p "$HOST_PORT:8080" \
    --ipc=host \
    --runtime=nvidia \
    cowdude/flapi

echo "Demo websocket client: http://localhost:$HOST_PORT"
echo "websocket endpoint:    ws://localhost:$HOST_PORT/v1/ws"
# Demo app (and API protocol) documentation below
```

---

## Build requirements (golang service)

- golang SDK >= 1.16.0 (required for `//go:embed <3`)

---

## Runtime requirements (golang service)

- Linux host machine, x86_64 (TODO: fix author's laziness to properly deal with endianess)
- docker
- nvidia runtime for docker (TODO: Dockerfile for CPU-only image)
- flashlight binaries, apps included
- ffmpeg

---

## Building the image

```sh
# get and update the sources (this repository)
go get -u github.com/cowdude/flapi/...
cd "$GOPATH/src/github.com/cowdude/flapi"

# build the server and the docker image (cold builds take a long, long time)
# Try proceeding to the next step as you wait, which will also take a while
docker build -t flapi .
```

---

## Downloading the models/assets

```sh
# prepare a persistent directory to store the model files.
# You'll need about 10GB, ideally on your fastest SSD for dev.
HOST_DATA="$HOME/flapi-data"
mkdir -p "$HOST_DATA"

# fetch the model and other required assets in $HOST_DATA
cd "$GOPATH/src/github.com/cowdude/flapi"
./download_models.sh "$HOST_DATA"
ls -lh "$HOST_DATA"

# copy the smoke-test sound file to $HOST_DATA
# (the server uses it to self-test at startup, and force some resource allocations...)
cp data/hello.wav "$HOST_DATA/"
```

---

## Running the service API

```sh
# define the port on which you want to expose the HTTP API
HOST_PORT=8080

# check your env vars
ls -lh "$HOST_DATA"
echo API endpoint: http://localhost:$HOST_PORT

# run the API
docker run \
  -v "$HOST_DATA:/data" \
  -p "$HOST_PORT:8080" \
  --ipc=host \
  --runtime=nvidia \
  flapi
```

---

## Using the demo browser app

1. start/run the FLAPI server container

1. Using a somewhat modern browser, go to `http://localhost:$HOST_PORT`

1. Wait for the engine to initialize (30s-5m - have yet another coffee)

1. Once the ASR engine is ready, click the `Toggle record` button on the right,
   say something, and click on it a second time to end the recording.

1. You should get something similar to this output:

```ruby
# NOTES: first line is the most recent entry ;
#  => start reading at the end of this code section
# - incoming text message (JSON-encoded) from server: < TXT; TEXT_MESSAGE_JSON_PAYLOAD
# - outgoing audio data blob from client: > BIN; BROWSER_AUDIO_MIMETYPE | BLOB_SIZE bytes

< TXT; {"event":"prediction","result":{"input_file":"/tmp/2.wav","text":"this is a test"}}
# recorder drained
> BIN; audio/webm;codecs=opus | 1107 bytes
# recorder state changed: recording -> inactive
> BIN; audio/webm;codecs=opus | 1359 bytes
[...]
< TXT; {"event":"prediction","result":{"input_file":"/tmp/1.wav","text":"hello github"}}
> BIN; audio/webm;codecs=opus | 1560 bytes
[...]
> BIN; audio/webm;codecs=opus | 1560 bytes
< TXT; {"event":"prediction","result":{"input_file":"/tmp/0.wav","text":""}}
> BIN; audio/webm;codecs=opus | 1560 bytes
[...]
> BIN; audio/webm;codecs=opus | 1339 bytes
# recorder state changed: inactive -> recording
< TXT; {"event":"status_changed","result":true,"message":"ASR ready"}
# websocket open
```

Here is a short explanation of what happens under the hood:

1. The client (web browser) sends webm audio chunks to the server.

1. The server buffers the audio stream and transcodes it to WAV/PCM 16bit 16kHz.

1. WAV Audio input is decoded for silence detection.

1. Audio input is split into segments every time it encounters a silence of >=300ms.

1. Each segment is then padded with the surrounding silence and saved to disk.

1. The server asks FL:ASR to process the audio segment file, and reads the prediction back.

Empty predictions may show up, usually because no speech was found in the given audio segment.
Those usually show up on my end, when:

- the brower/OS/driver/hardware audio recorder's black magic recalibrate its filters;
- your capture hardware has a cheap analog-to-digital converter (ADC) that eats tons of voltage spikes
- there is no audible _speech_, but we captured a quick, loud oscillation, such as:
  - mouse/keyboard sounds
  - tongue/breathing/coughing sounds.

Please see the Background section below for somewhat more in-depth implementation details
while I hopefully update this documentation.

---

## Server and Model Configuration

Service configuration is written in YAML:

```yaml
# flashlight ASR command-line options, mainly for models tuning. See links below.
flashlight:
  executable: /root/flashlight/build/bin/asr/fl_asr_tutorial_inference_ctc
  accoustic_model: /data/am_transformer_ctc_stride3_letters_300Mparams.bin
  language_model: /data/lm_common_crawl_large_4gram_prun0-0-5_200kvocab.bin
  tokens: /data/tokens.txt
  lexicon: /data/lexicon.txt
  beam_size: 100
  beam_size_token: 10
  beam_threshold: 100
  language_model_weight: 3.0
  word_score: 0.0

# the service runs a smoke-test given an audio file and expected output at startup.
# used to force GPU/CPU resource allocations on FLASR
warmup:
  audio: /data/hello.wav
  ground_truth: 'hello'
  repeat: 3

# the activity section lets you tweak how the service locates speech activity in the
# internal WAV/PCM audio stream
activity:
  # `threshold`: anything below this audio gain threshold is treated as silent.
  # lower values make the service more responsive to low-volume inputs, but
  # will also capture noise, and usually yield poor/empty predictions.
  # higher values make the system more resilient to background noise, at the cost of
  # potentially missing the start of some speech segments.
  threshold: -23dB
  # `timeout`: minimum silence duration between words/sentences.
  # lower if you want more predictions per second, or lower end-to-end response time
  # higher values tend to work best with a high flashlight.language_model_weight.
  timeout: 300ms
  # `buffer_duration`: audio ring-buffer size
  # increase if processing very long sentences
  buffer_duration: 10s
  # `gain_smooth`: don't change it.
  # it tweaks the reactivity of the audio gain EMA for y-shifting the audio input.
  gain_smooth: 0.97
  # `context_prefix`: duration of silence preceding speech activity that is fed to the ASR.
  # increase gently (probably up to ~500ms) if you are 'missing the start' of some words
  context_prefix: 150ms

# HTTP server config
http:
  listen: ':8080' # all ifaces, TCP 8080
```

See the [official flasr tutorial](https://github.com/facebookresearch/flashlight/tree/master/flashlight/app/asr/tutorial) for testing different models, finetuning, etc.

There is also [the official flashlight documentation](https://github.com/facebookresearch/flashlight/tree/master/flashlight/app/asr).

---

## WS API protocol

> **IMPORTANT**: While the server was made to support concurrent users, I haven't tested the current code
> in such use case, and I highly doubt that it will work as expected.

```js
// NOTE: this code section is read from top to bottom.

// new connection begins
// server sends text:
{ "event": "status_changed", "result": false, "message": "..." }

// status_changed is false => wait for next event
// [...]

// server sends text:
{ "event": "status_changed", "result": true, "message": "..." }

// client can now write audio data in a sequence of binary messages
// status_changed cannot become false anymore, no need to keep track of it

// send some media file containing at least an audio stream (like the output of a microphone capture device,
// or the contents of a video/audio file.
// audio can be split into multiple arbitrary-sized binary messages in order to stream the content
// of the request, and get results while you keep sending the following chunk(s)
// [insert audio data here, as *BINARY* message(s), NOT TEXT]

// wait for predictions
// [...]

// server sends a prediction
{"event": "prediction", "result": { "input_file": "/tmp/1.wav", "text": "hello github" } }
// [...]
// server sends another prediction
{"event": "prediction", "result": { "input_file": "/tmp/2.wav", "text": "you get the idea" } }
```

- The server always sends JSON-encoded text messages ;
- The server expects to receive only binary messages ;
- Connection is full-duplex: you can send audio data while receiving predictions ;
- The binary messages contain the ordered audio stream, such as the content an MP3-encoded file ;
- The client is allowed to stop/resume sending frames at any point after `status_changed` becomes `true` ;
- Sending aberrant volumes of data over an extended period of time will cause the server to fall behind
  and discard oldest audio data ;
- You can feed it anything that ffmpeg accepts as input audio stream ;
- Make sure to include the stream and codec format headers whenever possible ;
- The last audio blob should end with ~300ms of silence, in order to be fully processed and not hang
  in the server's audio buffer forever. This is due to the lack of client-server syncing. I'm fine with
  this limitation as of now.

---

## Background

The name _FLAPI_ stands for Flashlight-API, which is as exotic as its implementation. Note that
this project is not affiliated with Flashlight or Facebook AI research.
This project executes flashlight's **tutorial app** (formerly known as `wav2letter`)
and communicates through stdio. Sounds terrible, right? Well, that's just the tip.

On the _user side_ of the beast, we rely on `ffmpeg` for transcoding pretty much any
input stream into WAV/PCM. Casually pulling another extra 600MB docker layer.
I almost felt bad about it, so here's a refreshing perspective:

```
# du -sh /lib/x86_64-linux-gnu/*
[...]
1.4G	/lib/x86_64-linux-gnu/libcudnn_cnn_infer.so.8.0.5
2.3G	/lib/x86_64-linux-gnu/libcudnn_static_v8.a

# You're welcome.
```

Anyway, after experimenting for a week with my prototype - written in python back then,
I noticed this ASR model currently has [an issue](https://github.com/facebookresearch/flashlight/issues/265)
with long sentences. Even worse: longer inputs are more complex to process, overall resulting in
a poor real-time experience. And yeah, python.

Neuron activates: split the audio file in shorter audio files. Brain oofed a couple of hours later when
I realized the complexity of driving multiple ffmpeg processes and having no control on what was going on, or
how to improve the results.

Second impulse: rewrite the entire proto in go, but keep ffmpeg for transcoding into WAV/PCM.
Ended up having to refresh half of my rusty digital signal processing basics, while
carefully avoiding reading anything close to `FFT.c`. I'll probably rewrite the silence/activity
detection/chunking stuff anyway.

So here we are today. Without any optimization, the end-to-end latency is about `50ms + SpeechTime + NetworkDelay`
on my desktop. By being more aggressive on silence detection, it is possible to keep `SpeechTime` fairly low ;
therefore allowing processing of the previous segment while the user records the next one (i.e. double-buffering).
On the other hand, this means that we lose some context when sending the input (potentially a single word) to the ASR
model.

Having realtime output on top would be doable (i.e. streaming WAV/PCM to FL's ASR lib), but definitely not using flashlight's tutorial app.

As previously mentioned, I'd also like to implement concurrent requests, too. While I currently lack
performance/stability reports over extended time periods, I am certain that the process
CPU/GPU loads are low enough on my hardware to support concurrent requests against a single model.

I will also probably end up adding an HTTP POST endpoint serving the same goal as the websocket. I originally
went for websockets, because this was the easiest way to stream data to a python Flask app, that's it.

Stay tuned.

More random stuff worth noting:

- The audio captures I get from my cheap microphone are bad, downsampling it to 16kHz can make some voices
  harder to understand, especially "background speakers"
- Expect dirty results out of dirty inputs, especially when it comes to logscale data
- Finetuning the model will very likely increase accuracy and quality of predictions
- 16kHz (16000Hz) means you get 16 samples every millisecond
- the model and myself both perform better when given both past and future silent contexts
  (for example, 'one day' can end up sounding like 'wonder')
- a spectral approach would likely work well, too

---

## Credits

- ASR speech-to-text: Flashlight, Facebook research, https://github.com/facebookresearch/flashlight
- models and datasets: wav2letter, Facebook research, https://github.com/facebookresearch/wav2letter/tree/master/recipes/rasr
- The ffmpeg/libavconv project for one of the best oneliners, https://ffmpeg.org
- `hello.wav`, tim.kahn, https://freesound.org/people/tim.kahn/sounds/99471/
- https://stackoverflow.com/questions/4810841/pretty-print-json-using-javascript

---

## External documentation, references

- http://www-mmsp.ece.mcgill.ca/Documents/AudioFormats/WAVE/WAVE.html
- https://ffmpeg.org/ffmpeg-filters.html#silencedetect
- https://en.wikipedia.org/wiki/A-weighting
- https://manual.audacityteam.org/man/spectral_selection.html
