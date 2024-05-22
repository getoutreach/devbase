APP := devbase
OSS := true
_ := $(shell ./scripts/devbase.sh) 

include .bootstrap/root/Makefile

ORB_DEV_TAG ?= first

.PHONY: build-orb
pre-build:: build-orb
build-orb:
	circleci orb pack orbs/shared > orb.yml

.PHONY: validate-orb
validate-orb: build-orb
	circleci orb validate orb.yml

.PHONY: publish-orb
publish-orb: validate-orb
	circleci orb publish orb.yml dev@dev:$(ORB_DEV_TAG)

## <<Stencil::Block(targets)>>
ifeq ($(OS),Windows_NT)     # is Windows_NT on XP, 2000, 7, Vista, 10...
	detectedOS := Windows
else
	detectedOS := $(shell uname -s)
endif

ifeq ($(detectedOS),Darwin)
	# BSD sed
	SED_I := sed -i ""
else
	# GNU sed
	SED_I := sed -i
endif

ORB_DEV_TAG ?= first
STABLE_ORB_VERSION = $(shell gh release list --limit 1 --exclude-drafts --exclude-pre-releases --json name --jq '.[].name | ltrimstr("v")')

.PHONY: build-orb
pre-build:: build-orb

.PHONY: build-orb
build-orb:
	circleci orb pack orbs/shared > orb.yml

.PHONY: validate-orb
validate-orb: build-orb
	circleci orb validate orb.yml

.PHONY: publish-orb
publish-orb: validate-orb
	circleci orb publish orb.yml getoutreach/shared@dev:$(ORB_DEV_TAG)

post-stencil::
	$(SED_I) "s/dev:first/$(STABLE_ORB_VERSION)/" .circleci/config.yml
	yarn add --dev @getoutreach/semantic-release-circleci-orb
	./scripts/shell-wrapper.sh catalog-sync.sh
## <</Stencil::Block>>
