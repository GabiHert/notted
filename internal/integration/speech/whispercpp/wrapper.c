#include "wrapper.h"

#include <stdio.h>
#include <string.h>

#include "whisper.h"

// Only errors reach stderr, which the app forwards to its log.
static void phonon_whisper_log(enum ggml_log_level level, const char *text, void *user_data) {
	(void)user_data;
	if (level == GGML_LOG_LEVEL_ERROR) {
		fputs(text, stderr);
	}
}

struct whisper_context *phonon_whisper_init(const char *model_path, int use_gpu) {
	whisper_log_set(phonon_whisper_log, NULL);
	struct whisper_context_params cp = whisper_context_default_params();
	cp.use_gpu = use_gpu != 0;
	cp.flash_attn = use_gpu != 0;
	return whisper_init_from_file_with_params(model_path, cp);
}

void phonon_whisper_free(struct whisper_context *ctx) {
	if (ctx != NULL) {
		whisper_free(ctx);
	}
}

int phonon_whisper_full(struct whisper_context *ctx, const float *samples, int n,
                        const char *language, const char *vad_model_path, int threads) {
	// The same decoding as whisper-cli's defaults: beam search of 5, temperature fallback.
	struct whisper_full_params p = whisper_full_default_params(WHISPER_SAMPLING_BEAM_SEARCH);
	p.n_threads = threads;
	p.beam_search.beam_size = 5;
	p.greedy.best_of = 5;
	p.language = language;
	p.detect_language = false;
	p.translate = false;
	p.initial_prompt = NULL;
	p.no_context = true;
	p.print_special = false;
	p.print_progress = false;
	p.print_realtime = false;
	p.print_timestamps = false;
	if (vad_model_path != NULL && vad_model_path[0] != '\0') {
		p.vad = true;
		p.vad_model_path = vad_model_path;
		p.vad_params = whisper_vad_default_params();
	}
	return whisper_full(ctx, p, samples, n);
}

int phonon_whisper_n_segments(struct whisper_context *ctx) { return whisper_full_n_segments(ctx); }

int64_t phonon_whisper_segment_t0(struct whisper_context *ctx, int i) { return whisper_full_get_segment_t0(ctx, i); }

int64_t phonon_whisper_segment_t1(struct whisper_context *ctx, int i) { return whisper_full_get_segment_t1(ctx, i); }

const char *phonon_whisper_segment_text(struct whisper_context *ctx, int i) {
	return whisper_full_get_segment_text(ctx, i);
}
