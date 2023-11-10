APP := devbase
OSS := true
_ := $(shell ./scripts/devbase.sh) 

include .bootstrap/root/Makefile

## <<Stencil::Block(targets)>>
ORB_DEV_TAG ?= first

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
## <</Stencil::Block>>
