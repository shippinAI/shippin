# Shippin: each user's own AI CLI logins live on their AI home volume (~/.ai), which is theirs alone
# and follows them across their workspaces. ~/.ai/shared is for their own cross-AI state (Chatmaton).
export CLAUDE_CONFIG_DIR="$HOME/.ai/claude" CODEX_HOME="$HOME/.ai/codex" GROK_HOME="$HOME/.ai/grok"
export DISABLE_AUTOUPDATER=1
if [ -d "$HOME/.ai" ] && [ -w "$HOME/.ai" ]; then
  chmod 700 "$HOME/.ai" 2>/dev/null
  mkdir -p "$CLAUDE_CONFIG_DIR" "$CODEX_HOME" "$GROK_HOME" "$HOME/.ai/shared" 2>/dev/null
  [ -e "$CODEX_HOME/config.toml" ] || printf 'cli_auth_credentials_store = "file"\n' > "$CODEX_HOME/config.toml" 2>/dev/null
fi
