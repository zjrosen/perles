# Perles

![License](https://img.shields.io/github/license/zjrosen/perles)
![Last Commit](https://img.shields.io/github/last-commit/zjrosen/perles)
![Issues](https://img.shields.io/github/issues/zjrosen/perles)
![Stars](https://img.shields.io/github/stars/zjrosen/perles)

A terminal UI for [beads](https://github.com/steveyegge/beads) issue tracking powered by **BQL (Beads Query Language)**.

<p align="center">
  <img src="./docs/assets/board.png" width="1440" alt="kanban board">
</p>
<p align="center">
  <img src="./docs/assets/search.png" width="1440" alt="bql search">
</p>

## ✨ Features

- 🔍 **Boolean Search** — Search issues with AND, OR, NOT logic
- 📅 **Date Filtering** — Filter by creation date, due date, and more
- 🌳 **Dependency Trees** — Visualize and traverse issue relationships
- 📋 **Kanban Views** — Build custom board views for workflow management
- ⌨️ **Keyboard-First** — Navigate everything without leaving your terminal
- 🎨 **Custom Queries** — Save and reuse BQL queries for common searches

## 🚀 Quick Start

### Install Script

```bash
curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
```

### Homebrew (macOS/Linux)

```bash
# Add the tap
brew tap zjrosen/perles

# Install
brew install perles
```

### From Source

```bash
# Clone the repository
git clone https://github.com/zjrosen/perles.git
cd perles

# Build and install
cargo build --release
cp target/release/perles ~/.local/bin/
```

## 💡 Usage

Run `perles` in any directory containing a `.beads/` folder:

```bash
cd your-project
perles
```

## 📖 Documentation

Full documentation available at **[zjrosen.github.io/perles](https://zjrosen.github.io/perles/)**.

## 🔧 Requirements

- A beads or beads-rust enabled project with `.beads/` directory
- Minimum beads database version v0.62.0

  ```bash
  # Upgrade beads database
  bd migrate
  ```

## 🤝 Contributing

Contributions welcome! Please feel free to submit issues and pull requests.

## 📜 License

MIT License

---

README optimized with [Gingiris README Generator](https://gingiris.github.io/github-readme-generator/)
