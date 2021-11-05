APP := devbase
ORG := getoutreach
OSS := true

include root/Makefile


.PHONT: build-orb
build-orb:
	circleci orb pack orbs/shared > orb.yaml

.PHONY: validate-orb
validate-orb: build-orb
	circleci orb validate orb.yaml

.PHONY: publish-orb
publish-orb: validate-orb
	circleci orb publish orb.yaml getoutreach/shared@dev:first
