# contract_version: q-v1
# name: who-calls
sym $1 exact | edges callers depth=1 | read ctx=2
