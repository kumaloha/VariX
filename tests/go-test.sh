#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
varix_module_dir="$repo_root/varix"
eval_module_dir="$repo_root/eval"
varix_overlay="$(mktemp "${TMPDIR:-/tmp}/varix-test-overlay.XXXXXX")"
eval_overlay="$(mktemp "${TMPDIR:-/tmp}/varix-eval-test-overlay.XXXXXX")"
trap 'rm -f "$varix_overlay" "$eval_overlay"' EXIT

write_overlay() {
  local test_root="$1"
  local module_dir="$2"
  local overlay="$3"
  local exclude_root="${4:-}"
  printf '{"Replace":{'
  first=1
  while IFS= read -r -d '' test_file; do
    if [[ -n "$exclude_root" && "$test_file" == "$exclude_root/"* ]]; then
      continue
    fi
    rel="${test_file#$test_root/}"
    module_file="$module_dir/$rel"
    if [[ "$first" -eq 0 ]]; then
      printf ','
    fi
    first=0
    printf '"%s":"%s"' "$module_file" "$test_file"
  done < <(find "$test_root" -type f -name '*_test.go' -print0 2>/dev/null | sort -z)
  printf '}}'
}

write_overlay "$repo_root/tests" "$varix_module_dir" "$varix_overlay" "$repo_root/tests/eval" > "$varix_overlay"
write_overlay "$repo_root/tests/eval" "$eval_module_dir" "$eval_overlay" > "$eval_overlay"

if [[ "$#" -eq 0 ]]; then
  set -- ./...
fi

run_varix=1
run_eval=0
has_package_arg=0
has_varix_package_arg=0
for arg in "$@"; do
  case "$arg" in
    ./*)
      has_package_arg=1
      ;;
  esac
  case "$arg" in
    ./...)
      run_eval=1
      has_varix_package_arg=1
      ;;
    ./eval|./eval/...)
      run_eval=1
      ;;
    ./*)
      has_varix_package_arg=1
      ;;
  esac
done

if [[ "$has_package_arg" -eq 1 && "$has_varix_package_arg" -eq 0 ]]; then
  run_varix=0
fi

if [[ "$run_varix" -eq 1 ]]; then
  (
    cd "$varix_module_dir"
    go test -overlay="$varix_overlay" "$@"
  )
fi

if [[ "$run_eval" -eq 1 ]]; then
  eval_args=("$@")
  for i in "${!eval_args[@]}"; do
    case "${eval_args[$i]}" in
      ./eval)
        eval_args[$i]="."
        ;;
      ./eval/...)
        eval_args[$i]="./..."
        ;;
    esac
  done
  (
    cd "$eval_module_dir"
    go test -overlay="$eval_overlay" "${eval_args[@]}"
  )
fi
