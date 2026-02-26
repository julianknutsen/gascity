// Gas City hooks for OpenCode.
// Installed by gc into {workDir}/.opencode/plugins/gascity.js
//
// Events:
//   session.created    → gc prime (load context)
//   session.compacted  → gc prime (reload after compaction)
//   session.deleted    → gc hook --inject (pick up work on exit)
//   chat.system.transform → gc mail check --inject (inject mail per-turn)

const { execSync } = require("child_process");

function run(cmd) {
  try {
    return execSync(cmd, { encoding: "utf-8", timeout: 30000 }).trim();
  } catch {
    return "";
  }
}

module.exports = {
  name: "gascity",

  events: {
    "session.created": () => run("gc prime"),
    "session.compacted": () => run("gc prime"),
    "session.deleted": () => run("gc hook --inject"),
  },

  hooks: {
    "experimental.chat.system.transform": (system) => {
      const mail = run("gc mail check --inject");
      if (mail) {
        return system + "\n\n" + mail;
      }
      return system;
    },
  },
};
