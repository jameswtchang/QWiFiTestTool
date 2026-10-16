# Git & Commit Rules

- **Do NOT proactively commit changes to git.**
- **Only commit and push to GitHub when the user explicitly triggers the `/sync` workflow.**
- Do not run `git commit` or `git push` on your own initiative without explicit instructions.

# Deployment Rules

- **Do NOT manually run `go build` or `adb push` commands to deploy the application.**
- When recompilation and deployment to the board are needed, you MUST ask the user to execute the `/deploy` slash command.
- Exception: when the user triggers `/sync` and there are code changes, its hardware verification step may build and deploy (following `.claude/skills/deploy/SKILL.md`), because the user explicitly requested that workflow.
