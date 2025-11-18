## Running `kubectl-fzf-server` on macOS (via `launchd`)

This project includes a `launchd` service definition for macOS:

- `service/macos/com.github.rooty0.kubectl_fzf_server.plist`

This lets `kubectl-fzf-server` run in the background and restart automatically, similar to the systemd unit on Linux.

### 1. Adjust the plist (paths, `KUBECONFIG`, `PATH`)

Open the plist file and make sure these values are correct for your system:

```xml
<key>ProgramArguments</key>
<array>
  <!-- Path to your kubectl-fzf-server binary -->
  <string>/Users/YOURUSER/bin/kubectl-fzf-server</string>
</array>

<key>EnvironmentVariables</key>
<dict>
  <!-- Optional: custom kubeconfig -->
  <key>KUBECONFIG</key>
  <string>/Users/YOURUSER/.kube/config</string>

  <!-- Make sure this PATH includes the directory where kubectl is installed -->
  <key>PATH</key>
  <string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>
</dict>
```
Replace `YOURUSER` and any paths as needed.

### 2. Install the LaunchAgent

Copy the plist into your user LaunchAgents directory:
```shell
mkdir -p ~/Library/LaunchAgents
cp service/macos/com.github.rooty0.kubectl_fzf_server.plist \
   ~/Library/LaunchAgents/com.github.rooty0.kubectl_fzf_server.plist
```
### 3. Load and start it

Load the agent (this also starts it if `RunAtLoad` is set to `true` in the plist):
```shell
launchctl load ~/Library/LaunchAgents/com.github.rooty0.kubectl_fzf_server.plist
launchctl start com.github.rooty0.kubectl-fzf-server
```

### 4. Logs

By default, logs are written to:
```xml
<key>StandardOutPath</key>
<string>/Users/YOURUSER/Library/Logs/kubectl-fzf-server.log</string>
<key>StandardErrorPath</key>
<string>/Users/YOURUSER/Library/Logs/kubectl-fzf-server-error.log</string>
```
You can tail them with:
```shell
tail -f ~/Library/Logs/kubectl-fzf-server.log \
       ~/Library/Logs/kubectl-fzf-server-error.log
```
### 5. Unload (disable) the LaunchAgent

If you want to disable it:
```shell
launchctl unload ~/Library/LaunchAgents/com.github.rooty0.kubectl_fzf_server.plist
```