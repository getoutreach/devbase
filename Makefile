APP := devbase
OSS := true
_ := $(shell ./scripts/devbase.sh) 

include .bootstrap/root/Makefile

## <<Stencil::Block(targets)>>
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
	sed -i "s/dev:first/$(STABLE_ORB_VERSION)/" .circleci/config.yml
	yarn add --dev @getoutreach/semantic-release-circleci-orb
## <</Stencil::Block>>
