#!/usr/bin/env bash
# Build the pinned MAP-14 candidate. This does not certify ENGINE_GATE.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source_dir= sdk_dir= output_dir= abi=arm64-v8a jobs=3
usage() {
  echo "Usage: $0 --source DIR --sdk DIR --output NEW_DIR [--abi arm64-v8a|armeabi-v7a|x86_64] [--jobs N]" >&2
  exit 2
}
while (($#)); do
  case "$1" in
    --source|--sdk|--output|--abi|--jobs)
      (($# >= 2)) || usage
      case "$1" in
        --source) source_dir=$2 ;;
        --sdk) sdk_dir=$2 ;;
        --output) output_dir=$2 ;;
        --abi) abi=$2 ;;
        --jobs) jobs=$2 ;;
      esac
      shift 2 ;;
    *) usage ;;
  esac
done
[[ -n "$source_dir" && -n "$sdk_dir" && -n "$output_dir" ]] || usage
[[ "$jobs" =~ ^[1-9][0-9]*$ ]] || usage
case "$abi" in
  arm64-v8a) arch=arm64 ;;
  armeabi-v7a) arch=arm32 ;;
  x86_64) arch=x86_64 ;;
  *) usage ;;
esac
[[ ! -e "$output_dir" ]] || { echo "Output already exists; choose a new directory" >&2; exit 2; }
source_dir=$(realpath "$source_dir")
sdk_dir=$(realpath "$sdk_dir")
pin=4587b8ad4d12c9f59982709fdaf03cc925948827
[[ $(git -C "$source_dir" rev-parse HEAD) == "$pin" ]] || { echo "Wrong CoMaps source revision" >&2; exit 2; }
ndk="$sdk_dir/ndk/28.2.13676358"
[[ -x "$sdk_dir/cmake/3.22.1/bin/cmake" && -f "$ndk/source.properties" ]] || { echo "Pinned NDK/CMake missing" >&2; exit 2; }
[[ -n "${JAVA_HOME:-}" && -x "$JAVA_HOME/bin/javac" ]] || { echo "JAVA_HOME must point to a complete JDK 21" >&2; exit 2; }
"$JAVA_HOME/bin/javac" -version 2>&1 | grep -Eq '^javac 21([.]|$)' || { echo "JDK 21 required" >&2; exit 2; }
patch_file="$repo_root/maps/engine/upstream/ndk-r28-lld.patch"
if git -C "$source_dir" apply --check "$patch_file" 2>/dev/null; then
  git -C "$source_dir" apply "$patch_file"
elif ! git -C "$source_dir" apply --reverse --check "$patch_file" 2>/dev/null; then
  echo "Source differs from the recorded patch; preserve local changes and reconcile them first" >&2
  exit 2
fi
submodules=$(git -C "$source_dir" submodule status --recursive)
if grep -Eq '^[-+U]' <<< "$submodules"; then
  echo "Initialize all pinned submodules; mismatched revisions are refused" >&2
  exit 2
fi

work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT
printf '%s\n' "$submodules" > "$work_dir/submodules.txt"
git -C "$source_dir" diff --binary HEAD -- CMakeLists.txt cmake android/sdk > "$work_dir/runtime-source.diff"
export ANDROID_HOME="$sdk_dir" ANDROID_SDK_ROOT="$sdk_dir"
(
  cd "$source_dir/android"
  ./gradlew :sdk:assembleRelease "-P$arch" -PpropMinSdkVersion=26 "-Pnjobs=$jobs" \
    --offline --no-daemon --max-workers=2 --console=plain
) > "$work_dir/build.log" 2>&1 || { cat "$work_dir/build.log" >&2; exit 1; }

# The verifier publishes only the actual Gradle release AAR, never a debug AAR
# with a manually replaced .so. Invalid artifacts leave no output directory.
python3 "$repo_root/scripts/maps/verify-comaps-aar.py" \
  --aar "$source_dir/android/sdk/build/outputs/aar/sdk-release.aar" \
  --abi "$abi" --ndk "$ndk" --output "$output_dir"
cp "$work_dir/build.log" "$work_dir/submodules.txt" "$work_dir/runtime-source.diff" "$output_dir/"
cp "$patch_file" "$output_dir/"
printf 'Candidate ready: -PcomapsSdkDir=%s\nENGINE_GATE remains pending its other acceptance checks.\n' "$(realpath "$output_dir")"
