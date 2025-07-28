APP := devbase
OSS := true
_ := $(shell ./scripts/devbase.sh)

include .bootstrap/root/Makefile

## <<Stencil::Block(targets)>>
post-stencil::
	./scripts/shell-wrapper.sh catalog-sync.sh
	./scripts/shell-wrapper.sh circleci-orb-sync.sh
## <</Stencil::Block>>
