---
description: Node.js package management - install, run scripts, manage dependencies, npm commands.
metadata:
    nanogrip:
        requires:
            bins:
                - npm
                - node
name: npm
---

# NPM Skill

Use npm for Node.js package management and running scripts.

## Basic Commands

### Initialize project
```bash
npm init
npm init -y          # default values
npm init --scope=@myorg
```

### Install packages
```bash
# Local install
npm install lodash
npm i lodash
npm i lodash@4.17.21

# Global install
npm install -g typescript
npm i -g typescript

# Save to dependencies (default)
npm install lodash

# Save to devDependencies
npm install --save-dev typescript
npm i -D typescript

# Install from package.json
npm install
npm i
```

### Uninstall
```bash
npm uninstall lodash
npm remove lodash
npm rm lodash

# Remove from devDependencies
npm uninstall --save-dev typescript
```

### Update
```bash
npm update
npm update lodash
npm update -g typescript  # global
```

## Package.json Operations

### View installed packages
```bash
npm list
npm list --depth=0         # top-level only
npm list lodash            # specific package
npm list --global          # global packages
```

### Scripts
```bash
# Run script from package.json
npm run dev
npm run build
npm run test
npm run start

# List available scripts
npm run

# Run arbitrary command
npm run -- some-command
```

## Package Management

### View package info
```bash
npm view lodash
npm view lodash version
npm view lodash versions
npm view lodash dependencies
npm view lodash repo
```

### Search packages
```bash
npm search lodash
npm search --long
```

### Audit
```bash
npm audit
npm audit fix
npm audit fix --force
```

## Node Version Management

### Using nvm (Node Version Manager)
```bash
# Install nvm (first time)
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.0/install.sh | bash

# Install Node versions
nvm install 20
nvm install --lts

# Use specific version
nvm use 20
nvm default 20

# List installed versions
nvm ls

# List available versions
nvm ls-remote
```

### Using n (alternative)
```bash
npm install -g n
n 20
n lts
```

## Common Workflows

### Create new React app
```bash
npx create-react-app my-app
cd my-app
npm install
npm start
```

### Create new Node.js project
```bash
mkdir my-project
cd my-project
npm init -y
npm install express
npm install --save-dev nodemon
```

### Install and run
```bash
# Install dependencies
npm install

# Start development server
npm run dev

# Build for production
npm run build

# Start production server
npm start
```

## Global Packages

### Common global tools
```bash
npm install -g yarn           # package manager
npm install -g typescript     # language
npm install -g ts-node       # TypeScript runner
npm install -g nodemon       # dev server
npm install -g pm2           # process manager
npm install -g http-server   # static server
npm install -g serve         # static server
npm install -g eslint        # linter
npm install -g prettier      # formatter
```

### List global packages
```bash
npm list -g --depth=0
```

## Yarn Comparison

### Equivalent commands
```bash
npm install    # yarn
npm install    # yarn install
npm i lodash   # yarn add lodash
npm install -D # yarn add -D
npm install -g # yarn global
npm run test   # yarn test
npm run build  # yarn build
```

## PNPM (Alternative)

### Basic usage
```bash
npm install -g pnpm
pnpm install
pnpm add lodash
pnpm add -D typescript
```

## Publishing Packages

### Login
```bash
npm login
npm adduser
```

### Publish
```bash
npm publish
npm publish --access public  # scoped package
npm publish --tag beta      # tag version
```

## Tips

### Clean npm cache
```bash
npm cache clean --force
```

### Use with proxy
```bash
npm config set proxy http://proxy.example.com:8080
npm config set https-proxy http://proxy.example.com:8080
```

### Set registry
```bash
npm config set registry https://registry.npmjs.org/
npm config set registry https://registry.npmmirror.com  # China mirror
```
