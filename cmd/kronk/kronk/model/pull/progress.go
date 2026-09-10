package pull

import (
	"fmt"
	"io"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
)

type progressPrinter struct {
	w       io.Writer
	rewrite bool
	src     string
	open    bool
}

func (pp *progressPrinter) print(pr toolapp.PullResponse) {
	if pr.Progress == nil {
		pp.status(pr.Status)
		return
	}

	pp.progress(pr.Status, pr.Progress.Src, pr.Progress.Complete)
}

func (pp *progressPrinter) status(status string) {
	pp.close()
	fmt.Fprintln(pp.w, status)
}

func (pp *progressPrinter) progress(status string, src string, complete bool) {
	if !pp.rewrite {
		fmt.Fprintln(pp.w, status)
		return
	}

	if pp.open && src != pp.src {
		pp.close()
	}

	if !pp.open {
		pp.src = src
		pp.open = true
	}

	// Disable automatic wrapping while writing the update. This keeps the
	// progress display on one terminal row even when the status is wider than
	// the terminal; the right edge is clipped until a later update fits.
	fmt.Fprintf(pp.w, "\r\x1b[2K\x1b[?7l%s\x1b[?7h", status)

	if complete {
		pp.close()
	}
}

func (pp *progressPrinter) close() {
	if !pp.open {
		return
	}

	fmt.Fprintln(pp.w)
	pp.open = false
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
