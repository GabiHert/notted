// A small C layer over whisper.h for the Go package: it fills whisper_full_params so Go never
// touches that struct, and silences whisper.cpp's logging.
#ifndef PHONON_WHISPER_WRAPPER_H
#define PHONON_WHISPER_WRAPPER_H

#include <stdint.h>

struct whisper_context;

struct whisper_context *phonon_whisper_init(const char *model_path, int use_gpu);
void phonon_whisper_free(struct whisper_context *ctx);

// phonon_whisper_full transcribes n 16 kHz mono samples. language is a Whisper code ("pt", "en");
// vad_model_path, when not empty, turns on whisper.cpp's Silero VAD. Returns 0 on success.
int phonon_whisper_full(struct whisper_context *ctx, const float *samples, int n,
                        const char *language, const char *vad_model_path, int threads);

int phonon_whisper_n_segments(struct whisper_context *ctx);
// Segment times are in 10 ms units from the start of the input.
int64_t phonon_whisper_segment_t0(struct whisper_context *ctx, int i);
int64_t phonon_whisper_segment_t1(struct whisper_context *ctx, int i);
const char *phonon_whisper_segment_text(struct whisper_context *ctx, int i);

#endif
