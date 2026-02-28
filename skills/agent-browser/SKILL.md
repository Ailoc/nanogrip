---
name: agent-browser
description: Browser automation CLI for AI agents. Use when the user needs to interact with websites, including navigating pages, filling forms, clicking buttons, taking screenshots, extracting data, testing web apps, or automating any browser task. Triggers include requests to "open a website", "fill out a form", "click a button", "take a screenshot", "scrape data from a page", "test this web app", "login to a site", "automate browser actions", or any task requiring programmatic web interaction.
metadata:
    nanogrip:
        requires:
            bins:
                - npx
---

# Browser Automation with agent-browser

## IMPORTANT: How to invoke agent-browser

**ALL agent-browser commands MUST be prefixed with `npx`:**

```bash
npx agent-browser <command> [options]
```

The `npx` tool will automatically download and run agent-browser if it's not already installed. You do NOT need to manually install anything.

**❌ WRONG:**
```bash
agent-browser open https://example.com  # This will fail with "command not found"
```

**✅ CORRECT:**
```bash
npx agent-browser open https://example.com  # This works - npx handles installation
```

## Core Workflow

Every browser automation follows this pattern:

1. **Navigate**: `npx agent-browser open <url>`
2. **Snapshot**: `npx agent-browser snapshot -i` (get element refs like `@e1`, `@e2`)
3. **Interact**: Use refs to click, fill, select
4. **Re-snapshot**: After navigation or DOM changes, get fresh refs

```bash
# Navigate and snapshot
npx agent-browser open https://example.com/form
npx agent-browser snapshot -i
# Output: @e1 [input type="email"], @e2 [input type="password"], @e3 [button] "Submit"

# Fill form and submit
npx agent-browser fill @e1 "user@example.com"
npx agent-browser fill @e2 "password123"
npx agent-browser click @e3
npx agent-browser wait --load networkidle
npx agent-browser snapshot -i  # Check result
```

## Essential Commands

```bash
# ALWAYS use npx prefix!
npx agent-browser <command> [options]

# Navigation
npx agent-browser open <url>              # Navigate to URL
npx agent-browser close                   # Close browser

# Snapshot - get interactive elements with references
npx agent-browser snapshot -i             # Get @refs for interactive elements
npx agent-browser snapshot -i -C          # Include cursor-interactive elements
npx agent-browser snapshot -s "#selector" # Scope to CSS selector

# Interaction (use @refs from snapshot)
npx agent-browser click @e1               # Click element
npx agent-browser click @e1 --new-tab     # Click and open in new tab
npx agent-browser fill @e2 "text"         # Clear and type text
npx agent-browser type @e2 "text"         # Type without clearing
npx agent-browser select @e1 "option"     # Select dropdown option
npx agent-browser check @e1               # Check checkbox
npx agent-browser press Enter             # Press key
npx agent-browser scroll down 500         # Scroll page

# Get information
npx agent-browser get text @e1            # Get element text
npx agent-browser get url                 # Get current URL
npx agent-browser get title               # Get page title

# Wait
npx agent-browser wait @e1                # Wait for element
npx agent-browser wait --load networkidle # Wait for network idle
npx agent-browser wait 2000               # Wait milliseconds

# Capture
npx agent-browser screenshot              # Screenshot to temp dir
npx agent-browser screenshot --full       # Full page screenshot
npx agent-browser screenshot --annotate   # Annotated screenshot with numbered labels
npx agent-browser pdf output.pdf          # Save as PDF
```

## Command Chaining

Commands can be chained with `&&` in a single shell invocation for efficiency:

```bash
# Chain open + wait + snapshot
npx agent-browser open https://example.com && npx agent-browser wait --load networkidle && npx agent-browser snapshot -i

# Chain multiple interactions
npx agent-browser fill @e1 "user@example.com" && npx agent-browser fill @e2 "password123" && npx agent-browser click @e3
```

## Common Patterns

### Form Submission
```bash
npx agent-browser open https://example.com/signup
npx agent-browser snapshot -i
npx agent-browser fill @e1 "Jane Doe"
npx agent-browser fill @e2 "jane@example.com"
npx agent-browser select @e3 "California"
npx agent-browser check @e4
npx agent-browser click @e5
npx agent-browser wait --load networkidle
```

### Data Extraction
```bash
npx agent-browser open https://example.com/products
npx agent-browser snapshot -i
npx agent-browser get text @e5           # Get specific element text
```

### Taking Screenshots
```bash
npx agent-browser open https://example.com
npx agent-browser wait --load networkidle
npx agent-browser screenshot output.png
```

## What to Do When npx Downloads the Package

On first run, npx will show download messages like:
```
npm warn exec The following package was not found and will be installed:
npm warn deprecated agent-browser@0.15.1
```

This is NORMAL and EXPECTED. Just wait for the download to complete, then the command will execute automatically. The package is cached locally after the first download.

## Troubleshooting

| Error | Solution |
|-------|----------|
| `agent-browser: not found` | You forgot the `npx` prefix! Use `npx agent-browser` |
| `npx: command not found` | Install Node.js - `sudo apt install nodejs npm` |
| Browser doesn't launch | This is handled automatically by agent-browser |
| "package not found" | Wait for npx to finish downloading (it may take 10-30 seconds on first run) |
