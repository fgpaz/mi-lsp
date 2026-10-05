# contract_version: q-v1
# name: trace
sym $1 exact | read | edges callers depth=2 | edges callees depth=1
