package mdiff

import (
	"strings"
	"testing"
)

// ============================================================
// 真实语言 patch 测试 — 模拟 LLM 对 HTML/JS/CSS/Python 的修改
// ============================================================

// --------------- HTML ---------------

const htmlOriginal = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My App</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <div id="app">
        <h1>Hello World</h1>
        <p>Welcome to my app</p>
    </div>
    <script src="app.js"></script>
</body>
</html>
`

func TestHTML_AddNavAndFooter(t *testing.T) {
	patch := `--- index.html
+++ index.html
@@ -9,6 +9,12 @@
 <body>
     <div id="app">
+        <nav>
+            <a href="/">Home</a>
+            <a href="/about">About</a>
+        </nav>
         <h1>Hello World</h1>
         <p>Welcome to my app</p>
+        <footer>
+            <p>&copy; 2024</p>
+        </footer>
     </div>
`
	result, err := PatchText(htmlOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "<nav>") {
		t.Error("nav should be added")
	}
	if !strings.Contains(result, "<footer>") {
		t.Error("footer should be added")
	}
	if !strings.Contains(result, "<h1>Hello World</h1>") {
		t.Error("existing h1 should be preserved")
	}
}

func TestHTML_ChangeTitle(t *testing.T) {
	patch := `--- index.html
+++ index.html
@@ -4,3 +4,3 @@
     <meta charset="UTF-8">
     <meta name="viewport" content="width=device-width, initial-scale=1.0">
-    <title>My App</title>
+    <title>New App Title</title>
     <link rel="stylesheet" href="style.css">
`
	result, err := PatchText(htmlOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "<title>New App Title</title>") {
		t.Errorf("title not changed: got %q", result)
	}
}

func TestHTML_MultiHunk_HeadAndBody(t *testing.T) {
	patch := `--- index.html
+++ index.html
@@ -5,3 +5,4 @@
     <meta name="viewport" content="width=device-width, initial-scale=1.0">
-    <title>My App</title>
+    <title>Updated App</title>
+    <meta name="description" content="A great app">
     <link rel="stylesheet" href="style.css">
@@ -11,3 +12,3 @@
         <h1>Hello World</h1>
-        <p>Welcome to my app</p>
+        <p>Welcome to the updated app</p>
     </div>
`
	result, err := PatchText(htmlOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "<title>Updated App</title>") {
		t.Error("title not updated in head")
	}
	if !strings.Contains(result, `content="A great app"`) {
		t.Error("meta description not added")
	}
	if !strings.Contains(result, "Welcome to the updated app") {
		t.Error("paragraph not updated in body")
	}
}

func TestHTML_Roundtrip(t *testing.T) {
	newHTML := strings.Replace(htmlOriginal, "<title>My App</title>", "<title>Changed</title>", 1)
	newHTML = strings.Replace(newHTML, `<p>Welcome to my app</p>`, `<p>New content</p>`, 1)

	fd := DiffFiles("index.html", htmlOriginal, "index.html", newHTML, 3)
	patchText := FormatFileDiff(fd)
	t.Logf("HTML patch:\n%s", patchText)

	result, err := PatchText(htmlOriginal, patchText)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if result != newHTML {
		t.Errorf("roundtrip failed:\n  got:  %q\n  want: %q", result, newHTML)
	}
}

// --------------- JavaScript ---------------

const jsOriginal = `import React, { useState, useEffect } from 'react';
import axios from 'axios';

const API_URL = 'https://api.example.com';

function App() {
    const [data, setData] = useState([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    useEffect(() => {
        fetchData();
    }, []);

    const fetchData = async () => {
        try {
            const response = await axios.get(` + "`${API_URL}/items`" + `);
            setData(response.data);
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    };

    if (loading) return <div>Loading...</div>;
    if (error) return <div>Error: {error}</div>;

    return (
        <div className="app">
            <h1>Items</h1>
            <ul>
                {data.map(item => (
                    <li key={item.id}>{item.name}</li>
                ))}
            </ul>
        </div>
    );
}

export default App;
`

func TestJS_AddImportAndFunction(t *testing.T) {
	patch := `--- App.jsx
+++ App.jsx
@@ -1,4 +1,5 @@
 import React, { useState, useEffect } from 'react';
 import axios from 'axios';
+import { debounce } from 'lodash';
 
 const API_URL = 'https://api.example.com';
@@ -15,2 +16,7 @@
     const fetchData = async () => {
+        setLoading(true);
+        setError(null);
+    };
+
+    const debouncedFetch = debounce(async () => {
         try {
`
	result, err := PatchText(jsOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "import { debounce }") {
		t.Error("lodash import not added")
	}
	if !strings.Contains(result, "debouncedFetch") {
		t.Error("debouncedFetch function not added")
	}
}

func TestJS_ReplaceReturnJSX(t *testing.T) {
	patch := `--- App.jsx
+++ App.jsx
@@ -29,9 +29,14 @@
     return (
-        <div className="app">
-            <h1>Items</h1>
+        <div className="app container">
+            <header>
+                <h1>Items Dashboard</h1>
+                <button onClick={fetchData}>Refresh</button>
+            </header>
             <ul>
                 {data.map(item => (
-                    <li key={item.id}>{item.name}</li>
+                    <li key={item.id} className="item">
+                        <span>{item.name}</span>
+                        <span>{item.price}</span>
+                    </li>
                 ))}
`
	result, err := PatchText(jsOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, `className="app container"`) {
		t.Error("className not updated")
	}
	if !strings.Contains(result, "Items Dashboard") {
		t.Error("header text not updated")
	}
	if !strings.Contains(result, "item.price") {
		t.Error("price span not added")
	}
}

func TestJS_Roundtrip(t *testing.T) {
	newJS := strings.Replace(jsOriginal, "const [data, setData] = useState([]);",
		"const [data, setData] = useState([]);\n    const [search, setSearch] = useState('');", 1)
	newJS = strings.Replace(newJS, `setError(err.message);`, `setError(err.message || 'Unknown error');`, 1)

	fd := DiffFiles("App.jsx", jsOriginal, "App.jsx", newJS, 3)
	patchText := FormatFileDiff(fd)
	t.Logf("JS patch:\n%s", patchText)

	result, err := PatchText(jsOriginal, patchText)
	if err != nil {
		t.Fatalf("roundtrip error: %v", err)
	}
	if result != newJS {
		t.Errorf("JS roundtrip mismatch")
	}
}

// --------------- CSS ---------------

const cssOriginal = `.app {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
    font-family: Arial, sans-serif;
}

.header {
    background-color: #333;
    color: white;
    padding: 10px 20px;
    display: flex;
    justify-content: space-between;
    align-items: center;
}

.nav a {
    color: white;
    text-decoration: none;
    margin-left: 15px;
}

.nav a:hover {
    text-decoration: underline;
}

.content {
    padding: 20px 0;
}

.card {
    border: 1px solid #ddd;
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 16px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.card:hover {
    box-shadow: 0 4px 8px rgba(0,0,0,0.2);
}

.footer {
    text-align: center;
    padding: 20px;
    color: #666;
    border-top: 1px solid #eee;
}
`

func TestCSS_AddMediaQueryAndModify(t *testing.T) {
	patch := `--- style.css
+++ style.css
@@ -1,5 +1,6 @@
 .app {
-    max-width: 1200px;
+    max-width: 1400px;
+    width: 100%;
     margin: 0 auto;
     padding: 20px;
     font-family: Arial, sans-serif;
@@ -33,5 +34,5 @@
 .card {
     border: 1px solid #ddd;
-    border-radius: 8px;
+    border-radius: 12px;
     padding: 16px;
     margin-bottom: 16px;
`
	result, err := PatchText(cssOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "max-width: 1400px") {
		t.Error("max-width not updated")
	}
	if !strings.Contains(result, "width: 100%") {
		t.Error("width not added")
	}
	if !strings.Contains(result, "border-radius: 12px") {
		t.Error("border-radius not updated")
	}
}

func TestCSS_AppendNewRules(t *testing.T) {
	patch := `--- style.css
+++ style.css
@@ -43,4 +43,14 @@
 .footer {
     text-align: center;
     padding: 20px;
     color: #666;
     border-top: 1px solid #eee;
 }
+
+@media (max-width: 768px) {
+    .app {
+        padding: 10px;
+    }
+    .card {
+        margin-bottom: 8px;
+    }
+}
`
	result, err := PatchText(cssOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "@media (max-width: 768px)") {
		t.Error("media query not added")
	}
}

func TestCSS_Roundtrip(t *testing.T) {
	newCSS := strings.Replace(cssOriginal, "background-color: #333;", "background-color: #1a1a2e;", 1)
	newCSS = strings.Replace(newCSS, "box-shadow: 0 2px 4px rgba(0,0,0,0.1);",
		"box-shadow: 0 2px 4px rgba(0,0,0,0.1);\n    transition: box-shadow 0.3s ease;", 1)

	fd := DiffFiles("style.css", cssOriginal, "style.css", newCSS, 3)
	patchText := FormatFileDiff(fd)

	result, err := PatchText(cssOriginal, patchText)
	if err != nil {
		t.Fatalf("CSS roundtrip error: %v", err)
	}
	if result != newCSS {
		t.Errorf("CSS roundtrip mismatch")
	}
}

// --------------- Python ---------------

const pythonOriginal = `import os
import logging
from typing import List, Optional
from dataclasses import dataclass

logger = logging.getLogger(__name__)

@dataclass
class Config:
    host: str = "localhost"
    port: int = 8080
    debug: bool = False
    db_url: str = "sqlite:///app.db"

class Database:
    def __init__(self, config: Config):
        self.config = config
        self.connection = None
    
    def connect(self):
        logger.info(f"Connecting to {self.config.db_url}")
        self.connection = create_connection(self.config.db_url)
    
    def disconnect(self):
        if self.connection:
            self.connection.close()
            self.connection = None

    def query(self, sql: str, params: Optional[List] = None):
        if not self.connection:
            raise RuntimeError("Not connected")
        cursor = self.connection.cursor()
        cursor.execute(sql, params or [])
        return cursor.fetchall()

class Server:
    def __init__(self, config: Config):
        self.config = config
        self.db = Database(config)
        self.routes = {}
    
    def route(self, path: str):
        def decorator(func):
            self.routes[path] = func
            return func
        return decorator
    
    def start(self):
        self.db.connect()
        logger.info(f"Server starting on {self.config.host}:{self.config.port}")
    
    def stop(self):
        self.db.disconnect()
        logger.info("Server stopped")
`

func TestPython_AddMethodAndImport(t *testing.T) {
	patch := `--- server.py
+++ server.py
@@ -1,4 +1,5 @@
 import os
 import logging
+import asyncio
 from typing import List, Optional
 from dataclasses import dataclass
@@ -30,6 +31,13 @@
     def query(self, sql: str, params: Optional[List] = None):
         if not self.connection:
             raise RuntimeError("Not connected")
         cursor = self.connection.cursor()
         cursor.execute(sql, params or [])
         return cursor.fetchall()
+
+    def execute(self, sql: str, params: Optional[List] = None):
+        if not self.connection:
+            raise RuntimeError("Not connected")
+        cursor = self.connection.cursor()
+        cursor.execute(sql, params or [])
+        self.connection.commit()
`
	result, err := PatchText(pythonOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "import asyncio") {
		t.Error("asyncio import not added")
	}
	if !strings.Contains(result, "def execute(self") {
		t.Error("execute method not added")
	}
	if !strings.Contains(result, "self.connection.commit()") {
		t.Error("commit call not in execute")
	}
}

func TestPython_ModifyDecorator(t *testing.T) {
	patch := `--- server.py
+++ server.py
@@ -42,6 +42,10 @@
     def route(self, path: str):
-        def decorator(func):
-            self.routes[path] = func
-            return func
-        return decorator
+        def decorator(func):
+            self.routes[path] = {
+                'handler': func,
+                'methods': ['GET'],
+            }
+            return func
+        return decorator
`
	result, err := PatchText(pythonOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "'handler': func") {
		t.Error("handler dict not added")
	}
	if !strings.Contains(result, "'methods': ['GET']") {
		t.Error("methods not added")
	}
}

func TestPython_Roundtrip(t *testing.T) {
	newPy := strings.Replace(pythonOriginal,
		`    host: str = "localhost"`,
		`    host: str = "0.0.0.0"`, 1)
	newPy = strings.Replace(newPy,
		`    port: int = 8080`,
		`    port: int = 3000`, 1)
	newPy = strings.Replace(newPy,
		`    def stop(self):
        self.db.disconnect()
        logger.info("Server stopped")`,
		`    def stop(self):
        self.db.disconnect()
        logger.info("Server stopped")

    def restart(self):
        self.stop()
        self.start()`, 1)

	fd := DiffFiles("server.py", pythonOriginal, "server.py", newPy, 3)
	patchText := FormatFileDiff(fd)
	t.Logf("Python patch:\n%s", patchText)

	result, err := PatchText(pythonOriginal, patchText)
	if err != nil {
		t.Fatalf("Python roundtrip error: %v", err)
	}
	if result != newPy {
		t.Errorf("Python roundtrip mismatch")
	}
}

// --------------- 多文件多语言 ---------------

func TestMultiLang_HTMLJSCSSPython(t *testing.T) {
	files := map[string]string{
		"index.html": htmlOriginal,
		"App.jsx":    jsOriginal,
		"style.css":  cssOriginal,
		"server.py":  pythonOriginal,
	}

	// 对每个文件做一处修改
	changes := []FileChange{
		{
			Path:       "index.html",
			OldContent: htmlOriginal,
			NewContent: strings.Replace(htmlOriginal, "<title>My App</title>", "<title>Production App</title>", 1),
		},
		{
			Path:       "App.jsx",
			OldContent: jsOriginal,
			NewContent: strings.Replace(jsOriginal, "const [loading, setLoading] = useState(true);",
				"const [loading, setLoading] = useState(false);", 1),
		},
		{
			Path:       "style.css",
			OldContent: cssOriginal,
			NewContent: strings.Replace(cssOriginal, "max-width: 1200px;", "max-width: 960px;", 1),
		},
		{
			Path:       "server.py",
			OldContent: pythonOriginal,
			NewContent: strings.Replace(pythonOriginal, `debug: bool = False`, `debug: bool = True`, 1),
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchText := FormatMultiFileDiff(mfd)
	t.Logf("multi-lang patch:\n%s", patchText)

	parsed, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed.Files) != 4 {
		t.Fatalf("expected 4 files, got %d", len(parsed.Files))
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(parsed, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
		for _, r := range result.Results {
			if !r.Success {
				t.Logf("  %s: %s", r.Path, r.Error)
			}
		}
	}
	if result.Succeeded != 4 {
		t.Errorf("expected 4 successes, got %d", result.Succeeded)
	}

	// 验证每次修改
	if !strings.Contains(accessor.files["index.html"], "Production App") {
		t.Error("HTML title not changed")
	}
	if !strings.Contains(accessor.files["App.jsx"], "useState(false)") {
		t.Error("JS loading state not changed")
	}
	if !strings.Contains(accessor.files["style.css"], "max-width: 960px") {
		t.Error("CSS max-width not changed")
	}
	if !strings.Contains(accessor.files["server.py"], "debug: bool = True") {
		t.Error("Python debug not changed")
	}
}

// --------------- Go ---------------

const goOriginal = `package main

import (
	"fmt"
	"net/http"
)

func main() {
	http.HandleFunc("/", handleHome)
	http.HandleFunc("/api/users", handleUsers)
	fmt.Println("Server starting on :8080")
	http.ListenAndServe(":8080", nil)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello, World!")
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	users := []string{"Alice", "Bob", "Charlie"}
	for _, u := range users {
		fmt.Fprintf(w, "%s\n", u)
	}
}
`

func TestGo_AddHandlerAndMiddleware(t *testing.T) {
	patch := `--- main.go
+++ main.go
@@ -3,3 +3,5 @@
 import (
 	"fmt"
+	"log"
 	"net/http"
+	"time"
 )
@@ -8,4 +10,5 @@
 func main() {
 	http.HandleFunc("/", handleHome)
 	http.HandleFunc("/api/users", handleUsers)
+	http.HandleFunc("/api/health", handleHealth)
 	fmt.Println("Server starting on :8080")
@@ -23,2 +26,12 @@
 		fmt.Fprintf(w, "%s\n", u)
 	}
 }
+
+func handleHealth(w http.ResponseWriter, r *http.Request) {
+	w.Header().Set("Content-Type", "application/json")
+	fmt.Fprintf(w, ` + "`" + `{"status":"ok","time":"%s"}` + "`" + `, time.Now().Format(time.RFC3339))
+}
+
+func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
+	return func(w http.ResponseWriter, r *http.Request) {
+		log.Printf("%s %s", r.Method, r.URL.Path)
+		next(w, r)
+	}
+}
`
	result, err := PatchText(goOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, `"log"`) {
		t.Error("log import not added")
	}
	if !strings.Contains(result, "handleHealth") {
		t.Error("health handler not added")
	}
	if !strings.Contains(result, "loggingMiddleware") {
		t.Error("middleware not added")
	}
}

func TestGo_Roundtrip(t *testing.T) {
	newGo := strings.Replace(goOriginal, `":8080"`, `":3000"`, -1)
	newGo = strings.Replace(newGo, `"Hello, World!"`, `"Welcome!"`, 1)

	fd := DiffFiles("main.go", goOriginal, "main.go", newGo, 3)
	patchText := FormatFileDiff(fd)

	result, err := PatchText(goOriginal, patchText)
	if err != nil {
		t.Fatalf("Go roundtrip error: %v", err)
	}
	if result != newGo {
		t.Errorf("Go roundtrip mismatch")
	}
}

// --------------- TypeScript ---------------

const tsOriginal = `interface User {
    id: number;
    name: string;
    email: string;
    role: 'admin' | 'user';
}

interface ApiResponse<T> {
    data: T;
    status: number;
    message: string;
}

class UserService {
    private baseUrl: string;
    private cache: Map<number, User>;

    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
        this.cache = new Map();
    }

    async getUser(id: number): Promise<User> {
        if (this.cache.has(id)) {
            return this.cache.get(id)!;
        }
        const response = await fetch(` + "`${this.baseUrl}/users/${id}`" + `);
        const result: ApiResponse<User> = await response.json();
        this.cache.set(id, result.data);
        return result.data;
    }

    async listUsers(): Promise<User[]> {
        const response = await fetch(` + "`${this.baseUrl}/users`" + `);
        const result: ApiResponse<User[]> = await response.json();
        return result.data;
    }
}

export { UserService, User, ApiResponse };
`

func TestTS_AddInterfaceField(t *testing.T) {
	patch := `--- user.ts
+++ user.ts
@@ -1,6 +1,8 @@
 interface User {
     id: number;
     name: string;
     email: string;
+    avatar?: string;
+    createdAt: Date;
     role: 'admin' | 'user';
 }
`
	result, err := PatchText(tsOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, "avatar?: string") {
		t.Error("avatar field not added")
	}
	if !strings.Contains(result, "createdAt: Date") {
		t.Error("createdAt field not added")
	}
}

func TestTS_Roundtrip(t *testing.T) {
	newTS := strings.Replace(tsOriginal, "private cache: Map<number, User>;",
		"private cache: Map<number, User>;\n    private ttl: number;", 1)
	newTS = strings.Replace(newTS, `role: 'admin' | 'user';`,
		`role: 'admin' | 'user' | 'moderator';`, 1)

	fd := DiffFiles("user.ts", tsOriginal, "user.ts", newTS, 3)
	patchText := FormatFileDiff(fd)

	result, err := PatchText(tsOriginal, patchText)
	if err != nil {
		t.Fatalf("TS roundtrip error: %v", err)
	}
	if result != newTS {
		t.Errorf("TS roundtrip mismatch")
	}
}

// --------------- JSON/YAML config ---------------

const jsonOriginal = `{
    "name": "my-project",
    "version": "1.0.0",
    "scripts": {
        "dev": "vite",
        "build": "vite build",
        "test": "vitest"
    },
    "dependencies": {
        "react": "^18.2.0",
        "react-dom": "^18.2.0"
    },
    "devDependencies": {
        "vite": "^5.0.0",
        "vitest": "^1.0.0"
    }
}
`

func TestJSON_AddDependency(t *testing.T) {
	patch := `--- package.json
+++ package.json
@@ -9,3 +9,4 @@
     "dependencies": {
         "react": "^18.2.0",
-        "react-dom": "^18.2.0"
+        "react-dom": "^18.2.0",
+        "axios": "^1.6.0"
     },
`
	result, err := PatchText(jsonOriginal, patch)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if !strings.Contains(result, `"axios": "^1.6.0"`) {
		t.Error("axios dependency not added")
	}
}
