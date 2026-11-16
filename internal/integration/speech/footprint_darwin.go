package speech

/*
#include <libproc.h>
#include <unistd.h>

static unsigned long long phonon_phys_footprint(void) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(getpid(), RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) return 0;
	return ri.ri_phys_footprint;
}
*/
import "C"

// footprint is this process's physical footprint in bytes, the number Activity Monitor shows as Memory.
func footprint() uint64 { return uint64(C.phonon_phys_footprint()) }
