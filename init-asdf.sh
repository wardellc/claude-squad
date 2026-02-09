#!/usr/bin/env bash
set -eu

if ! command -v asdf &>/dev/null; then
  echo "asdf not found on PATH"
  echo "Install asdf: https://asdf-vm.com/guide/getting-started.html"
  if [[ "$OSTYPE" == "darwin"* ]]; then
    echo
    echo "brew install asdf"
    echo
    echo "Note: you still need to set up shell integration per the above docs"
  fi
  exit 1
else
  echo "asdf is installed"
fi

if [[ ":$PATH:" != *":$HOME/.asdf/shims:"* ]]; then
  echo "The asdf shim directory ($HOME/.asdf/shims) was not found on your PATH"
  echo
  echo "You may need to add lines to your .zshrc/.bashrc etc per instructions at https://asdf-vm.com/guide/getting-started.html#_3-install-asdf"
  exit 1
else
  echo "The asdf shim directory is on the PATH"
fi

echo "Installing required asdf plugins..."

install_asdf_plugin() {
  local PLUGIN_NAME="$1"
  local PLUGIN_URL="${2:-}"
  if ! asdf plugin list | grep -q "$PLUGIN_NAME"; then
    echo "Installing plugin $PLUGIN_NAME..."
    if [ -z "$PLUGIN_URL" ]; then
      asdf plugin add "$PLUGIN_NAME"
    else
      asdf plugin add "$PLUGIN_NAME" "$PLUGIN_URL"
    fi
    echo "Plugin $PLUGIN_NAME has been installed."
  else
    echo "Plugin $PLUGIN_NAME is already installed."
  fi
}

install_asdf_plugin golang

echo "Running asdf install..."
set +e
asdf install
EXIT_STATUS=$?
set -e
if [[ $EXIT_STATUS -ne 0 ]]; then
  echo "asdf install was not successful"
  exit 1
else
  echo "asdf install ran successfully."
fi
echo
echo "asdf is ready to use."
