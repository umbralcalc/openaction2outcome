# openaction2outcome — offline mint + publish workflow.
# Override the rclone remote name with: make publish RCLONE_REMOTE=myremote
CLI    := go run ./cmd/openaction2outcome
SEAM   ?= floor-standards
REMOTE ?= r2

.PHONY: all fetch build validate score test publish verify clean

all: build validate          ## fetch (implicit) + mint + validate

fetch:                       ## download frozen inputs into data/cache (verify SHA-256)
	$(CLI) fetch

build:                       ## mint the mark + stage the episode sidecar (auto-fetches)
	$(CLI) build --seam $(SEAM)

validate:                    ## check every mark against the schema
	$(CLI) validate

score:                       ## score the committed example submission
	$(CLI) score --submission examples/submission.example.json --out scores/example.scores.json

study:                       ## run the calibration study (plug-in vs SBI coverage of truth)
	$(CLI) study --out scores/calibration-study.json

test:                        ## unit + real-data integration tests (offline)
	go test ./...

publish: build               ## upload staged artifacts to R2, then verify
	RCLONE_REMOTE=$(REMOTE) ./scripts/publish.sh

verify:                      ## verify live artifacts resolve + match hashes (no upload)
	RCLONE_REMOTE=$(REMOTE) ./scripts/publish.sh --verify-only

clean:                       ## remove staged upload artifacts
	rm -rf dist
