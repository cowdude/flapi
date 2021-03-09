package main

import (
	"flag"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/cowdude/flapi/src/audio"

	yaml "gopkg.in/yaml.v2"
)

var Config struct {
	Flashlight struct {
		Executable          string
		AccousticModel      string `yaml:"accoustic_model"`
		LanguageModel       string `yaml:"language_model"`
		Tokens              string
		Lexicon             string
		BeamSize            int     `yaml:"beam_size"`             //The number of top hypothesis to preserve at each decoding step
		BeamSizeToken       int     `yaml:"beam_size_token"`       //The number of top by acoustic model scores tokens set to be considered at each decoding step
		BeamThreshold       int     `yaml:"beam_threshold"`        //Cut of hypothesis far away by the current score from the best hypothesis
		LanguageModelWeight float64 `yaml:"language_model_weight"` //Language model weight to accumulate with acoustic model score
		WordScore           float64 `yaml:"word_score"`            //Score to add when word finishes (lexicon-based beam search decoder only)
	}
	HTTP struct {
		Listen string
	}
	Warmup *struct {
		Audio       string
		GroundTruth string `yaml:"ground_truth"`
		Repeat      int
	}

	Activity audio.ActivityOpts
}

var configPath = flag.String("config", "config.yml", "Path to config.yml file")

func LoadConfig() {
	if *configPath == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	f, err := os.Open(*configPath)
	if err != nil {
		log.Fatal("Failed to open config file: ", err)
	}
	defer f.Close()

	if err = yaml.NewDecoder(f).Decode(&Config); err != nil {
		log.Fatal("Failed to parse config file: ", err)
	}
}
