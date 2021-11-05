APP := devbase
ORG := getoutreach
OSS := true

include root/Makefile


.PHONT: build-orb
build-orb:
	circleci orb pack orbs/shared > orb.yaml
