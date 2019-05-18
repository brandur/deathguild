package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/brandur/modulir"
	"github.com/joeshaw/envdecode"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Main
//
//
//
//////////////////////////////////////////////////////////////////////////////

func main() {
	var rootCmd = &cobra.Command{
		Use:   "dg",
		Short: "dg is a static site generator for deathguild.brandur.org",
		Long: strings.TrimSpace(`
Sorg is a static site generator for the meta-Deathguild site
https://deathguild.brandur.org which contains Spotify playlists
and various other features.`),
	}

	buildCommand := &cobra.Command{
		Use:   "build",
		Short: "Run a single build loop",
		Long: strings.TrimSpace(`
Starts the build loop that watches for local changes and runs
when they're detected. A webserver is started on PORT (default
5004).`),
		Run: func(cmd *cobra.Command, args []string) {
			modulir.Build(getModulirConfig(), build)
		},
	}
	rootCmd.AddCommand(buildCommand)

	loopCommand := &cobra.Command{
		Use:   "loop",
		Short: "Start build and serve loop",
		Long: strings.TrimSpace(`
Runs the build loop one time and places the result in TARGET_DIR
(default ./public/).`),
		Run: func(cmd *cobra.Command, args []string) {
			modulir.BuildLoop(getModulirConfig(), build)
		},
	}
	rootCmd.AddCommand(loopCommand)

	if err := envdecode.Decode(&conf); err != nil {
		fmt.Fprintf(os.Stderr, "Error decoding conf from env: %v", err)
		os.Exit(1)
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing command: %v", err)
		os.Exit(1)
	}
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Variables
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Left as a global for now for the sake of convenience, but it's not used in
// very many places and can probably be refactored as a local if desired.
var conf Conf

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Types
//
//
//
//////////////////////////////////////////////////////////////////////////////

// Conf contains configuration information for the command. It's extracted
// from environment variables.
type Conf struct {
	// Concurrency is the number of build Goroutines that will be used to
	// fetch information over HTTP.
	Concurrency int `env:"CONCURRENCY,default=10"`

	// DatabaseURL is a connection string for a database used to store
	// playlist and song information.
	DatabaseURL string `env:"DATABASE_URL,required"`

	// DGEnv is the environment to run the app with. Use "development" to
	// activate development features.
	DGEnv string `env:"DG_ENV"`

	// GoogleAnalyticsID is the account identifier for Google Analytics to use.
	GoogleAnalyticsID string `env:"GOOGLE_ANALYTICS_ID"`

	// LocalFonts starts using locally downloaded versions of Google Fonts.
	// This is not ideal for real deployment because you won't be able to
	// leverage Google's CDN and the caching that goes with it, and may not get
	// the font format for requesting browsers, but good for airplane rides
	// where you otherwise wouldn't have the fonts.
	LocalFonts bool `env:"LOCAL_FONTS,default=false"`

	// Port is the port on which to serve HTTP when looping in development.
	Port int `env:"PORT,default=5004"`

	// SpotifyUser is the name of the Spotify user who owns the Death Guild
	// playlists. This is used to generate links.
	SpotifyUser string `env:"SPOTIFY_USER,required"`

	// TargetDir is the target location where the site will be built to.
	TargetDir string `env:"TARGET_DIR,default=./public"`

	// Verbose is whether the program will print debug output as it's running.
	Verbose bool `env:"VERBOSE,default=false"`
}

//////////////////////////////////////////////////////////////////////////////
//
//
//
// Private
//
//
//
//////////////////////////////////////////////////////////////////////////////

const (
	dgEnvDevelopment = "development"
)

func getLog() modulir.LoggerInterface {
	log := logrus.New()

	if conf.Verbose {
		log.SetLevel(logrus.DebugLevel)
	} else {
		log.SetLevel(logrus.InfoLevel)
	}

	return log
}

// getModulirConfig interprets Conf to produce a configuration suitable to pass
// to a Modulir build loop.
func getModulirConfig() *modulir.Config {
	return &modulir.Config{
		Concurrency: conf.Concurrency,
		Log:         getLog(),
		Port:        conf.Port,
		SourceDir:   ".",
		TargetDir:   conf.TargetDir,
		Websocket:   conf.DGEnv == dgEnvDevelopment,
	}
}
