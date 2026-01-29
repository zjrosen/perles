import express from 'express';
import cors from 'cors';
import fs from 'fs';
import path from 'path';
import readline from 'readline';
import os from 'os';

const app = express();
app.use(cors());
app.use(express.json());

const SESSIONS_BASE = path.join(os.homedir(), '.perles', 'sessions');

// List all sessions in the base directory
app.get('/api/sessions', async (req, res) => {
  try {
    if (!fs.existsSync(SESSIONS_BASE)) {
      return res.json({ basePath: SESSIONS_BASE, apps: [] });
    }

    const apps = [];
    const appDirs = fs.readdirSync(SESSIONS_BASE, { withFileTypes: true })
      .filter(d => d.isDirectory())
      .map(d => d.name);

    for (const appName of appDirs) {
      const appPath = path.join(SESSIONS_BASE, appName);
      const app = { name: appName, dates: [] };

      // Get date directories
      const dateDirs = fs.readdirSync(appPath, { withFileTypes: true })
        .filter(d => d.isDirectory() && /^\d{4}-\d{2}-\d{2}$/.test(d.name))
        .map(d => d.name)
        .sort()
        .reverse();

      for (const dateDir of dateDirs) {
        const datePath = path.join(appPath, dateDir);
        const dateEntry = { date: dateDir, sessions: [] };

        // Get session directories
        const sessionDirs = fs.readdirSync(datePath, { withFileTypes: true })
          .filter(d => d.isDirectory())
          .map(d => d.name);

        for (const sessionId of sessionDirs) {
          const sessionPath = path.join(datePath, sessionId);
          const metadataPath = path.join(sessionPath, 'metadata.json');
          
          let metadata = null;
          if (fs.existsSync(metadataPath)) {
            try {
              const content = fs.readFileSync(metadataPath, 'utf-8');
              if (content.trim()) {
                metadata = JSON.parse(content);
              }
            } catch (e) {
              // Ignore parse errors
            }
          }

          dateEntry.sessions.push({
            id: sessionId,
            path: sessionPath,
            startTime: metadata?.start_time || null,
            status: metadata?.status || 'unknown',
            workerCount: metadata?.workers?.length || 0,
            clientType: metadata?.client_type || 'unknown',
          });
        }

        // Sort sessions by start time (newest first)
        dateEntry.sessions.sort((a, b) => {
          if (!a.startTime) return 1;
          if (!b.startTime) return -1;
          return new Date(b.startTime).getTime() - new Date(a.startTime).getTime();
        });

        if (dateEntry.sessions.length > 0) {
          app.dates.push(dateEntry);
        }
      }

      if (app.dates.length > 0) {
        apps.push(app);
      }
    }

    // Sort apps alphabetically
    apps.sort((a, b) => a.name.localeCompare(b.name));

    res.json({ basePath: SESSIONS_BASE, apps });
  } catch (err) {
    console.error('Error listing sessions:', err);
    res.status(500).json({ error: err.message });
  }
});

// Load session from a directory path
app.post('/api/load-session', async (req, res) => {
  const { path: sessionPath } = req.body;
  
  if (!sessionPath) {
    return res.status(400).json({ error: 'Path is required' });
  }

  try {
    // Check if directory exists
    if (!fs.existsSync(sessionPath)) {
      return res.status(404).json({ error: 'Directory not found' });
    }

    const session = await loadSession(sessionPath);
    res.json(session);
  } catch (err) {
    console.error('Error loading session:', err);
    res.status(500).json({ error: err.message });
  }
});

async function loadSession(sessionPath) {
  const result = {
    path: sessionPath,
    metadata: null,
    fabric: [],
    mcpRequests: [],
    commands: [],
    messages: [],
    coordinator: { messages: [], raw: [] },
    workers: {},
    accountabilitySummary: null
  };

  // Load metadata.json
  const metadataPath = path.join(sessionPath, 'metadata.json');
  if (fs.existsSync(metadataPath)) {
    const content = fs.readFileSync(metadataPath, 'utf-8');
    if (content.trim()) {
      result.metadata = JSON.parse(content);
    }
  }

  // Load accountability_summary.md
  const summaryPath = path.join(sessionPath, 'accountability_summary.md');
  if (fs.existsSync(summaryPath)) {
    result.accountabilitySummary = fs.readFileSync(summaryPath, 'utf-8');
  }

  // Load fabric.jsonl
  const fabricPath = path.join(sessionPath, 'fabric.jsonl');
  if (fs.existsSync(fabricPath)) {
    result.fabric = await loadJsonl(fabricPath);
  }

  // Load mcp_requests.jsonl
  const mcpPath = path.join(sessionPath, 'mcp_requests.jsonl');
  if (fs.existsSync(mcpPath)) {
    result.mcpRequests = await loadJsonl(mcpPath);
  }

  // Load commands.jsonl
  const commandsPath = path.join(sessionPath, 'commands.jsonl');
  if (fs.existsSync(commandsPath)) {
    result.commands = await loadJsonl(commandsPath);
  }

  // Load messages.jsonl
  const messagesPath = path.join(sessionPath, 'messages.jsonl');
  if (fs.existsSync(messagesPath)) {
    result.messages = await loadJsonl(messagesPath);
  }

  // Load coordinator logs
  const coordinatorPath = path.join(sessionPath, 'coordinator');
  if (fs.existsSync(coordinatorPath)) {
    const coordMessagesPath = path.join(coordinatorPath, 'messages.jsonl');
    if (fs.existsSync(coordMessagesPath)) {
      result.coordinator.messages = await loadJsonl(coordMessagesPath);
    }
    const coordRawPath = path.join(coordinatorPath, 'raw.jsonl');
    if (fs.existsSync(coordRawPath)) {
      result.coordinator.raw = await loadJsonl(coordRawPath);
    }
  }

  // Load worker logs
  const workersPath = path.join(sessionPath, 'workers');
  if (fs.existsSync(workersPath)) {
    const workerDirs = fs.readdirSync(workersPath, { withFileTypes: true })
      .filter(d => d.isDirectory())
      .map(d => d.name);

    for (const workerDir of workerDirs) {
      const workerPath = path.join(workersPath, workerDir);
      result.workers[workerDir] = { messages: [], raw: [] };

      const workerMessagesPath = path.join(workerPath, 'messages.jsonl');
      if (fs.existsSync(workerMessagesPath)) {
        result.workers[workerDir].messages = await loadJsonl(workerMessagesPath);
      }
      const workerRawPath = path.join(workerPath, 'raw.jsonl');
      if (fs.existsSync(workerRawPath)) {
        result.workers[workerDir].raw = await loadJsonl(workerRawPath);
      }
      const workerSummaryPath = path.join(workerPath, 'accountability_summary.md');
      if (fs.existsSync(workerSummaryPath)) {
        result.workers[workerDir].accountabilitySummary = fs.readFileSync(workerSummaryPath, 'utf-8');
      }
    }
  }

  return result;
}

async function loadJsonl(filePath) {
  const lines = [];
  
  // Check if file is empty first
  const stats = fs.statSync(filePath);
  if (stats.size === 0) {
    return lines;
  }
  
  const fileStream = fs.createReadStream(filePath);
  const rl = readline.createInterface({ input: fileStream, crlfDelay: Infinity });

  for await (const line of rl) {
    if (line.trim()) {
      try {
        lines.push(JSON.parse(line));
      } catch (e) {
        // Skip malformed lines
        console.warn(`Skipping malformed JSON in ${filePath}: ${line.substring(0, 100)}`);
      }
    }
  }
  return lines;
}

const PORT = 3001;
app.listen(PORT, () => {
  console.log(`Server running on http://localhost:${PORT}`);
});
