package infra

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/GabiHert/phonon/internal/integration/speech"
)

// TranscribeWorkerArg is the first argument that makes the Phonon binary run as the transcribe worker.
const TranscribeWorkerArg = "transcribe-worker"

// RunTranscribeWorker serves transcription on stdin and stdout until stdin closes.
func RunTranscribeWorker(args []string) int {
	fs := flag.NewFlagSet(TranscribeWorkerArg, flag.ContinueOnError)
	models := fs.String("models", "", "speech models folder")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *models == "" {
		fmt.Fprintln(os.Stderr, "phonon transcribe-worker: --models is required")
		return 2
	}
	eng := speech.New(speech.Config{ModelsDir: *models})
	defer eng.Unload()
	if err := speech.ServeWorker(os.Stdin, os.Stdout, eng); err != nil {
		fmt.Fprintf(os.Stderr, "phonon transcribe-worker: %v\n", err)
		return 1
	}
	return 0
}

// transcribeWorker runs the speech model in a child Phonon, so its memory goes back when it stops.
func transcribeWorker(modelsDir string) *speech.ProcessEngine {
	return &speech.ProcessEngine{
		Files: speech.Files{ModelsDir: modelsDir},
		Command: func() *exec.Cmd {
			exe, err := os.Executable()
			if err != nil {
				exe = os.Args[0]
			}
			return exec.Command(exe, TranscribeWorkerArg, "--models", modelsDir)
		},
	}
}
