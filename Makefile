# Builds void-init and void-initfs in three modes:
#
#   debug           dynamically linked, unstripped (default `go build`)
#   release         dynamically linked, stripped (-ldflags="-s -w")
#   release-static  statically linked (CGO_ENABLED=0), stripped
#
# Usage:
#   make                 # all binaries, all modes
#   make debug           # both binaries, debug mode
#   make release         # both binaries, release mode
#   make release-static  # both binaries, release-static mode
#   make clean

GO       := go
BINARIES := void-init void-initfs
BUILDDIR := build
STRIP_LDFLAGS := -s -w

.PHONY: all debug release release-static release-archive clean FORCE

all: debug release release-static release-archive

debug: $(BINARIES:%=$(BUILDDIR)/debug/usr/local/bin/%)
release: $(BINARIES:%=$(BUILDDIR)/release/usr/local/bin/%)
release-static: $(BINARIES:%=$(BUILDDIR)/release-static/usr/local/bin/%)

$(BUILDDIR)/debug/usr/local/bin/%: FORCE
	CGO_ENABLED=1 $(GO) build -o $@ ./cmd/$*

$(BUILDDIR)/release/usr/local/bin/%: FORCE
	CGO_ENABLED=1 $(GO) build -ldflags="$(STRIP_LDFLAGS)" -o $@ ./cmd/$*

# CGO_ENABLED=0 alone is enough for a fully static binary here - this
# module has no cgo code anywhere, so there's no need for
# -extldflags=-static, which would otherwise require static libc
# archives that aren't installed by default on most distros (Arch
# included).
$(BUILDDIR)/release-static/usr/local/bin/%: FORCE
	CGO_ENABLED=0 $(GO) build -ldflags="$(STRIP_LDFLAGS)" -o $@ ./cmd/$*

FORCE:

clean:
	rm -rf $(BUILDDIR)
