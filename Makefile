APP := devbase
OSS := true
_ := $(shell ./scripts/devbase.sh)

include .bootstrap/root/Makefile

## <<Stencil::Block(targets)>>
STABLE_ORB_VERSION = $(shell gh release list --limit 1 --exclude-drafts --exclude-pre-releases --json name --jq '.[].name | ltrimstr("v")')

post-stencil::
	perl -p -i -e "s/dev:first/$(STABLE_ORB_VERSION)/g" .circleci/config.yml
	./scripts/shell-wrapper.sh catalog-sync.sh
## <</Stencil::Block>>
