.PHONY: fmt
fmt:
	@make -f root/Makefile fmt

.PHONY: test
test:
	@make -f root/Makefile test

.PHONY: dep
dep:
	@make -f root/Makefile dep
