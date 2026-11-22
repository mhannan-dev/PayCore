FORGE := forge
RPC_URL := https://rpc-test.qday.info
ACCOUNT ?=

.PHONY: build test fmt fork-test deploy-aevum

build:
	$(FORGE) build

test:
	$(FORGE) test -vv

fmt:
	$(FORGE) fmt

fork-test:
	$(FORGE) test --profile fork --match-path 'test/fork/*' --fork-url $(RPC_URL) --evm-version shanghai -vv

deploy-aevum:
ifndef ACCOUNT
	$(error Set ACCOUNT=<foundry-keystore-name> when calling make deploy-aevum)
endif
	$(FORGE) script script/Deploy.s.sol:Deploy \
		--legacy --broadcast --rpc-url $(RPC_URL) --account $(ACCOUNT) --evm-version shanghai
