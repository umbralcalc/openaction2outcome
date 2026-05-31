# openaction2outcome — offline mint + publish workflow.
# Override the rclone remote name with: make publish RCLONE_REMOTE=myremote
CLI    := go run ./cmd/openaction2outcome
SERIES ?= floor-standards
REMOTE ?= r2

.PHONY: all fetch build build-all validate score study test publish verify clean

all: build-all validate      ## fetch (implicit) + mint every series + validate

fetch:                       ## download frozen inputs into data/cache (verify SHA-256)
	$(CLI) fetch

build:                       ## mint one series (SERIES=floor-standards|shmi|bathing-water) + stage its sidecar
	$(CLI) build --series $(SERIES)

build-all:                   ## mint every series
	$(CLI) build --series floor-standards
	$(CLI) build --series shmi
	$(CLI) build --series bathing-water

validate:                    ## check every mark against the schema
	$(CLI) validate

score:                       ## score the committed example submission
	$(CLI) score --submission examples/submission.example.json --out scores/example.scores.json

study:                       ## run the calibration study (plug-in vs SBI coverage of truth)
	$(CLI) study --out scores/calibration-study.json

test:                        ## unit + real-data integration tests (offline)
	go test ./...

publish: build-all           ## upload staged artifacts to R2, then verify
	RCLONE_REMOTE=$(REMOTE) ./scripts/publish.sh

verify:                      ## verify live artifacts resolve + match hashes (no upload)
	RCLONE_REMOTE=$(REMOTE) ./scripts/publish.sh --verify-only

clean:                       ## remove staged upload artifacts
	rm -rf dist

hf:                          ## export a Hugging Face-ready dataset dir (dist/hf)
	$(CLI) export
