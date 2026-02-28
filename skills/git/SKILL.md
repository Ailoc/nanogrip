---
description: Git version control operations - clone, commit, push, pull, branch, merge, log, diff, stash, etc.
metadata:
    NanoGrip:
        requires:
            bins:
                - git
name: git
---

# Git Skill

Use git for version control operations. All commands should be run in a git repository directory.

## Basic Operations

### Clone a repository
```bash
git clone <repository-url>
git clone https://github.com/owner/repo.git
```

### Check status
```bash
git status
git status -s  # short format
```

### View differences
```bash
git diff                    # unstaged changes
git diff --staged          # staged changes
git diff HEAD~1..HEAD       # between commits
git diff main..branch       # between branches
```

### View history
```bash
git log
git log --oneline -10      # compact format, last 10
git log --graph --oneline  # visual graph
git log --author="name"    # filter by author
git log --since="2024-01-01"  # filter by date
```

## Branch Operations

### List branches
```bash
git branch                 # local
git branch -r              # remote
git branch -a             # all
```

### Create and switch branches
```bash
git checkout -b new-branch        # create and switch
git switch -c new-branch          # alternative (Git 2.23+)
git checkout branch-name          # switch
git switch branch-name            # alternative
```

### Delete branches
```bash
git branch -d branch-name    # safe delete (merged)
git branch -D branch-name   # force delete
git push origin --delete branch-name  # remote delete
```

## Staging and Committing

### Stage changes
```bash
git add filename              # specific file
git add .                    # all changes
git add -p                   # interactive staging
git add -u                   # tracked files only
```

### Commit changes
```bash
git commit -m "commit message"
git commit -am "message"     # add + commit (tracked files)
git commit --amend           # modify last commit
git commit --amend --no-edit  # same message
```

## Remote Operations

### Sync with remote
```bash
git fetch origin             # download without merging
git pull origin branch       # fetch + merge
git push origin branch       # upload
git push -u origin branch    # push and set upstream
```

### Manage remotes
```bash
git remote -v                # list remotes
git remote add origin url    # add remote
git remote remove origin    # remove remote
```

## Stash Operations

### Save and restore changes
```bash
git stash                   # save uncommitted changes
git stash push -m "message" # with message
git stash list              # list stashes
git stash pop               # apply and remove latest
git stash apply            # apply (keep stash)
git stash drop             # remove stash
git stash clear            # remove all
```

## Merge and Rebase

### Merge branches
```bash
git merge branch-name
git merge --no-ff branch-name  # no fast-forward
git merge --abort              # cancel merge
```

### Rebase (linear history)
```bash
git rebase main
git rebase -i HEAD~3          # interactive rebase
git rebase --continue         # after resolving
git rebase --abort            # cancel rebase
```

## Advanced Operations

### Reset and restore
```bash
git reset --soft HEAD~1      # undo commit, keep changes
git reset --mixed HEAD~1     # default, keep staging
git reset --hard HEAD~1      # discard all changes
git restore filename          # discard file changes
git restore --staged filename # unstage
```

### Cherry-pick
```bash
git cherry-pick commit-hash
git cherry-pick --no-commit commit-hash
```

### Tags
```bash
git tag v1.0.0                # create tag
git tag -a v1.0.0 -m "msg"   # annotated tag
git push origin tag-name      # push tag
git push origin --tags        # push all
```

### Worktree
```bash
git worktree add ../dir branch
git worktree list
git worktree remove ../dir
```
