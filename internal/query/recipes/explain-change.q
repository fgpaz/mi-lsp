# contract_version: q-v1
# name: explain-change
diff ref=$1 | edges callers depth=2 | docs
