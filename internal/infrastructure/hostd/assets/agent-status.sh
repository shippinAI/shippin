# Prints the build agent's state as one JSON object (M4). No pane contents, prompt or file names leave
# the workspace: only booleans, a timestamp and a count. $SESSION is set by shippin-hostd.
S=$HOME/.shippin
alive=false; tmux has-session -t "$SESSION" 2>/dev/null && alive=true
act=$(tmux display -p -t "$SESSION" '#{session_activity}' 2>/dev/null); case "$act" in ''|*[!0-9]*) act=0 ;; esac
exited=false; [ -e "$S/agent.exit" ] && exited=true
rc=$(awk '{print $2}' "$S/agent.exit" 2>/dev/null); case "$rc" in ''|*[!0-9]*) rc=-1 ;; esac
D=$(cat "$S/project_dir" 2>/dev/null || echo "$HOME/code")
n=0
if [ -e "$S/prompt.md" ]; then
  n=$(find "$D" -xdev -type f -newer "$S/prompt.md" -not -path '*/node_modules/*' -not -path '*/.git/*' \
    -not -path '*/.venv/*' 2>/dev/null | head -n 100000 | wc -l)
fi
printf '{"session":%s,"activity":%s,"exited":%s,"exit_code":%s,"files_changed":%s}\n' "$alive" "$act" "$exited" "$rc" "$n"
