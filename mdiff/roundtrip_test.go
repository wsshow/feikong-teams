package mdiff

import (
	"fmt"
	"strings"
	"testing"
)

// ============================================================
// 1. 基础往返一致性（Round-trip）测试
// ============================================================

func TestRoundTripSimple(t *testing.T) {
	roundTripTest(t, "line1\nline2\nline3\n", "line1\nchanged\nline3\n")
}

func TestRoundTripInsertOnly(t *testing.T) {
	roundTripTest(t, "a\nb\n", "a\nnew\nb\n")
}

func TestRoundTripDeleteOnly(t *testing.T) {
	roundTripTest(t, "a\nb\nc\n", "a\nc\n")
}

func TestRoundTripCompleteRewrite(t *testing.T) {
	roundTripTest(t, "old1\nold2\nold3\n", "new1\nnew2\nnew3\nnew4\n")
}

func TestRoundTripLargeFile(t *testing.T) {
	var oldLines, newLines []string
	for i := 0; i < 100; i++ {
		oldLines = append(oldLines, fmt.Sprintf("line %d", i))
		if i == 30 || i == 70 {
			newLines = append(newLines, fmt.Sprintf("changed %d", i))
		} else if i == 50 {
			continue
		} else {
			newLines = append(newLines, fmt.Sprintf("line %d", i))
		}
	}
	newLines = append(newLines, "appended line")

	oldContent := strings.Join(oldLines, "\n") + "\n"
	newContent := strings.Join(newLines, "\n") + "\n"
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripEmptyLines(t *testing.T) {
	roundTripTest(t, "a\n\nb\n\nc\n", "a\n\nB\n\nc\n")
}

func TestRoundTripSingleLineFile(t *testing.T) {
	roundTripTest(t, "old\n", "new\n")
}

func TestRoundTripWithEmptyLines(t *testing.T) {
	oldContent := "def func1():\n    pass\n\n\ndef func2():\n    old\n\n\ndef func3():\n    pass\n"
	newContent := "def func1():\n    pass\n\n\ndef func2():\n    new\n\n\ndef func3():\n    pass\n"
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripMultipleEmptyLines(t *testing.T) {
	roundTripTest(t, "a\n\n\n\nb\n", "a\n\n\n\nc\n")
}

// ============================================================
// 2. Multi-Hunk 往返测试
// ============================================================

func TestRoundTripMultiHunkRealWorld(t *testing.T) {
	oldContent := `package handler

import (
	"fmt"
	"net/http"
)

func HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	data := fetchData(id)
	fmt.Fprintf(w, "data: %s", data)
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	if body == "" {
		http.Error(w, "empty body", 400)
		return
	}
	saveData(body)
	w.WriteHeader(201)
}

func HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", 400)
		return
	}
	deleteData(id)
	w.WriteHeader(204)
}
`
	newContent := `package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	data := fetchData(id)
	json.NewEncoder(w).Encode(data)
}

func HandlePost(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	if body == "" {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	saveData(body)
	w.WriteHeader(http.StatusCreated)
}

func HandleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	deleteData(id)
	w.WriteHeader(http.StatusNoContent)
}
`
	roundTripTest(t, oldContent, newContent)
}

// ============================================================
// 3. Python 代码往返测试
// ============================================================

func TestRoundTripPythonMultiFunctionEdit(t *testing.T) {
	oldContent := `import os
import sys
from typing import List, Optional


def read_config(path: str) -> dict:
    """读取配置文件"""
    with open(path, 'r') as f:
        return json.load(f)


def process_data(data: List[dict]) -> List[dict]:
    """处理数据"""
    result = []
    for item in data:
        if item.get('active'):
            result.append(item)
    return result


def save_output(data: List[dict], path: str) -> None:
    """保存输出"""
    with open(path, 'w') as f:
        json.dump(data, f)


def main():
    config = read_config('config.json')
    data = process_data(config['data'])
    save_output(data, 'output.json')
    print("Done")


if __name__ == '__main__':
    main()
`
	newContent := `import os
import sys
import logging
from typing import List, Optional

logger = logging.getLogger(__name__)


def read_config(path: str) -> dict:
    """读取配置文件"""
    logger.info(f"Reading config from {path}")
    with open(path, 'r') as f:
        return json.load(f)


def process_data(data: List[dict], filter_key: str = 'active') -> List[dict]:
    """处理数据，支持自定义过滤键"""
    result = []
    for item in data:
        if item.get(filter_key):
            result.append(item)
    logger.info(f"Processed {len(result)}/{len(data)} items")
    return result


def save_output(data: List[dict], path: str) -> None:
    """保存输出"""
    with open(path, 'w') as f:
        json.dump(data, f, indent=2)
    logger.info(f"Saved {len(data)} items to {path}")


def main():
    logging.basicConfig(level=logging.INFO)
    config = read_config('config.json')
    data = process_data(config['data'])
    save_output(data, 'output.json')
    logger.info("Done")


if __name__ == '__main__':
    main()
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripPythonClassEdit(t *testing.T) {
	oldContent := `class UserService:
    def __init__(self, db):
        self.db = db

    def get_user(self, user_id: int):
        return self.db.query(User).filter_by(id=user_id).first()

    def create_user(self, name: str, email: str):
        user = User(name=name, email=email)
        self.db.add(user)
        self.db.commit()
        return user

    def update_user(self, user_id: int, **kwargs):
        user = self.get_user(user_id)
        for key, value in kwargs.items():
            setattr(user, key, value)
        self.db.commit()
        return user

    def delete_user(self, user_id: int):
        user = self.get_user(user_id)
        self.db.delete(user)
        self.db.commit()
`
	newContent := `class UserService:
    def __init__(self, db):
        self.db = db
        self.cache = {}

    def get_user(self, user_id: int):
        if user_id in self.cache:
            return self.cache[user_id]
        user = self.db.query(User).filter_by(id=user_id).first()
        if user:
            self.cache[user_id] = user
        return user

    def create_user(self, name: str, email: str):
        user = User(name=name, email=email)
        self.db.add(user)
        self.db.commit()
        self.cache[user.id] = user
        return user

    def update_user(self, user_id: int, **kwargs):
        user = self.get_user(user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        for key, value in kwargs.items():
            setattr(user, key, value)
        self.db.commit()
        self.cache[user_id] = user
        return user

    def delete_user(self, user_id: int):
        user = self.get_user(user_id)
        if not user:
            raise ValueError(f"User {user_id} not found")
        self.db.delete(user)
        self.db.commit()
        self.cache.pop(user_id, None)
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripPythonIndentationHeavy(t *testing.T) {
	oldContent := `def complex_handler(request):
    if request.method == 'POST':
        data = request.json

        if 'items' in data:
            for item in data['items']:
                if item.get('type') == 'A':
                    process_type_a(item)

                elif item.get('type') == 'B':
                    process_type_b(item)

                else:
                    log.warning(f"Unknown type: {item.get('type')}")

        return {'status': 'ok'}

    return {'error': 'method not allowed'}
`
	newContent := `def complex_handler(request):
    if request.method == 'POST':
        data = request.json

        if 'items' not in data:
            return {'error': 'items required'}

        results = []
        for item in data['items']:
            if item.get('type') == 'A':
                result = process_type_a(item)

            elif item.get('type') == 'B':
                result = process_type_b(item)

            else:
                log.warning(f"Unknown type: {item.get('type')}")
                continue

            results.append(result)

        return {'status': 'ok', 'results': results}

    return {'error': 'method not allowed'}
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripPythonLLMStyleDiff(t *testing.T) {
	oldContent := "def greet(name):  \n    msg = f\"Hello, {name}!\"  \n    print(msg)  \n    return msg  \n"
	newContent := "def greet(name):\n    msg = f\"Hi, {name}!\"\n    print(msg)\n    return msg\n"

	fd := DiffFiles("app.py", oldContent, "app.py", newContent, 3)
	patchStr := FormatFileDiff(fd)

	parsed, err := ParseFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	result, err := ApplyFileDiff(oldContent, parsed)
	if err != nil {
		t.Fatalf("apply error: %v", err)
	}
	if result != newContent {
		t.Errorf("round-trip mismatch:\ngot:  %q\nwant: %q", result, newContent)
	}
}

// ============================================================
// 4. HTML/CSS/JS/TS/Vue 代码往返测试
// ============================================================

func TestRoundTripHTMLMultiSectionEdit(t *testing.T) {
	oldContent := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>My App</title>
    <link rel="stylesheet" href="style.css">
</head>
<body>
    <header>
        <h1>Welcome</h1>
        <nav>
            <a href="/">Home</a>
            <a href="/about">About</a>
        </nav>
    </header>

    <main>
        <section class="hero">
            <h2>Hello World</h2>
            <p>This is a simple page.</p>
        </section>

        <section class="content">
            <div class="card">
                <h3>Card Title</h3>
                <p>Card content here.</p>
            </div>
        </section>
    </main>

    <footer>
        <p>&copy; 2024 My App</p>
    </footer>

    <script src="app.js"></script>
</body>
</html>
`
	newContent := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>My App - Dashboard</title>
    <link rel="stylesheet" href="style.css">
    <link rel="stylesheet" href="dashboard.css">
</head>
<body>
    <header>
        <h1>Dashboard</h1>
        <nav>
            <a href="/">Home</a>
            <a href="/dashboard">Dashboard</a>
            <a href="/about">About</a>
        </nav>
    </header>

    <main>
        <section class="hero">
            <h2>Welcome Back</h2>
            <p>Here is your dashboard overview.</p>
        </section>

        <section class="content">
            <div class="card">
                <h3>Statistics</h3>
                <p>Total users: <span id="user-count">0</span></p>
            </div>
            <div class="card">
                <h3>Recent Activity</h3>
                <ul id="activity-list"></ul>
            </div>
        </section>
    </main>

    <footer>
        <p>&copy; 2025 My App. All rights reserved.</p>
    </footer>

    <script src="app.js"></script>
    <script src="dashboard.js"></script>
</body>
</html>
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripCSSMultiRuleEdit(t *testing.T) {
	oldContent := `:root {
    --primary: #3498db;
    --secondary: #2ecc71;
    --bg: #ffffff;
    --text: #333333;
}

body {
    font-family: Arial, sans-serif;
    background-color: var(--bg);
    color: var(--text);
    margin: 0;
    padding: 0;
}

.container {
    max-width: 1200px;
    margin: 0 auto;
    padding: 20px;
}

.header {
    background-color: var(--primary);
    color: white;
    padding: 10px 20px;
}

.card {
    border: 1px solid #ddd;
    border-radius: 4px;
    padding: 16px;
    margin-bottom: 16px;
}

.btn {
    display: inline-block;
    padding: 8px 16px;
    background-color: var(--primary);
    color: white;
    border: none;
    border-radius: 4px;
    cursor: pointer;
}

.footer {
    background-color: #f5f5f5;
    padding: 20px;
    text-align: center;
}
`
	newContent := `:root {
    --primary: #6366f1;
    --secondary: #10b981;
    --bg: #f8fafc;
    --text: #1e293b;
    --border: #e2e8f0;
    --shadow: 0 1px 3px rgba(0, 0, 0, 0.1);
}

body {
    font-family: 'Inter', system-ui, sans-serif;
    background-color: var(--bg);
    color: var(--text);
    margin: 0;
    padding: 0;
    line-height: 1.6;
}

.container {
    max-width: 1280px;
    margin: 0 auto;
    padding: 24px;
}

.header {
    background-color: var(--primary);
    color: white;
    padding: 12px 24px;
    box-shadow: var(--shadow);
}

.card {
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 20px;
    margin-bottom: 20px;
    box-shadow: var(--shadow);
    transition: transform 0.2s ease;
}

.card:hover {
    transform: translateY(-2px);
}

.btn {
    display: inline-block;
    padding: 10px 20px;
    background-color: var(--primary);
    color: white;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-weight: 500;
    transition: background-color 0.2s ease;
}

.btn:hover {
    background-color: #4f46e5;
}

.footer {
    background-color: #f1f5f9;
    padding: 24px;
    text-align: center;
    border-top: 1px solid var(--border);
}
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripJavaScriptMultiFunctionEdit(t *testing.T) {
	oldContent := `import { useState } from 'react';

function fetchData(url) {
    return fetch(url)
        .then(res => res.json())
        .catch(err => console.error(err));
}

function formatDate(date) {
    return date.toLocaleDateString();
}

function UserList({ users }) {
    return (
        <ul>
            {users.map(user => (
                <li key={user.id}>{user.name}</li>
            ))}
        </ul>
    );
}

export default function App() {
    const [users, setUsers] = useState([]);

    const loadUsers = () => {
        fetchData('/api/users').then(data => setUsers(data));
    };

    return (
        <div>
            <h1>User Management</h1>
            <button onClick={loadUsers}>Load Users</button>
            <UserList users={users} />
        </div>
    );
}
`
	newContent := `import { useState, useEffect, useCallback } from 'react';

async function fetchData(url, options = {}) {
    try {
        const res = await fetch(url, options);
        if (!res.ok) throw new Error(` + "`HTTP ${res.status}`" + `);
        return await res.json();
    } catch (err) {
        console.error('Fetch error:', err);
        throw err;
    }
}

function formatDate(date, locale = 'zh-CN') {
    return new Intl.DateTimeFormat(locale, {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
    }).format(date);
}

function UserList({ users, onSelect }) {
    if (users.length === 0) {
        return <p>No users found.</p>;
    }
    return (
        <ul>
            {users.map(user => (
                <li key={user.id} onClick={() => onSelect(user)}>
                    {user.name} - {formatDate(new Date(user.createdAt))}
                </li>
            ))}
        </ul>
    );
}

export default function App() {
    const [users, setUsers] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const loadUsers = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const data = await fetchData('/api/users');
            setUsers(data);
        } catch (err) {
            setError(err.message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        loadUsers();
    }, [loadUsers]);

    return (
        <div>
            <h1>User Management</h1>
            {error && <p style={{ color: 'red' }}>{error}</p>}
            <button onClick={loadUsers} disabled={loading}>
                {loading ? 'Loading...' : 'Refresh'}
            </button>
            <UserList users={users} onSelect={u => console.log(u)} />
        </div>
    );
}
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripTypeScriptMultiEdit(t *testing.T) {
	oldContent := `interface User {
    id: number;
    name: string;
    email: string;
}

interface ApiResponse<T> {
    data: T;
    status: number;
}

class UserRepository {
    private baseUrl: string;

    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
    }

    async getUser(id: number): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async listUsers(): Promise<User[]> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `);
        const json: ApiResponse<User[]> = await res.json();
        return json.data;
    }

    async createUser(user: Omit<User, 'id'>): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(user),
        });
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }
}

export { UserRepository };
export type { User, ApiResponse };
`
	newContent := `interface User {
    id: number;
    name: string;
    email: string;
    role: 'admin' | 'user' | 'guest';
    createdAt: string;
}

interface PaginatedResponse<T> {
    data: T[];
    total: number;
    page: number;
    pageSize: number;
}

interface ApiResponse<T> {
    data: T;
    status: number;
    message?: string;
}

class UserRepository {
    private baseUrl: string;
    private token: string | null;

    constructor(baseUrl: string, token?: string) {
        this.baseUrl = baseUrl;
        this.token = token ?? null;
    }

    private getHeaders(): HeadersInit {
        const headers: HeadersInit = { 'Content-Type': 'application/json' };
        if (this.token) {
            headers['Authorization'] = ` + "`Bearer ${this.token}`" + `;
        }
        return headers;
    }

    async getUser(id: number): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `, {
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to get user: ${res.status}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async listUsers(page = 1, pageSize = 20): Promise<PaginatedResponse<User>> {
        const url = ` + "`${this.baseUrl}/users?page=${page}&pageSize=${pageSize}`" + `;
        const res = await fetch(url, {
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to list users: ${res.status}`" + `);
        return await res.json();
    }

    async createUser(user: Omit<User, 'id' | 'createdAt'>): Promise<User> {
        const res = await fetch(` + "`${this.baseUrl}/users`" + `, {
            method: 'POST',
            headers: this.getHeaders(),
            body: JSON.stringify(user),
        });
        if (!res.ok) throw new Error(` + "`Failed to create user: ${res.status}`" + `);
        const json: ApiResponse<User> = await res.json();
        return json.data;
    }

    async deleteUser(id: number): Promise<void> {
        const res = await fetch(` + "`${this.baseUrl}/users/${id}`" + `, {
            method: 'DELETE',
            headers: this.getHeaders(),
        });
        if (!res.ok) throw new Error(` + "`Failed to delete user: ${res.status}`" + `);
    }
}

export { UserRepository };
export type { User, ApiResponse, PaginatedResponse };
`
	roundTripTest(t, oldContent, newContent)
}

func TestRoundTripVueComponentEdit(t *testing.T) {
	oldContent := `<template>
  <div class="todo-app">
    <h1>Todo List</h1>
    <input v-model="newTodo" @keyup.enter="addTodo" placeholder="Add a todo">
    <ul>
      <li v-for="todo in todos" :key="todo.id">
        {{ todo.text }}
        <button @click="removeTodo(todo.id)">Delete</button>
      </li>
    </ul>
  </div>
</template>

<script>
export default {
  data() {
    return {
      newTodo: '',
      todos: [],
    };
  },
  methods: {
    addTodo() {
      if (this.newTodo.trim()) {
        this.todos.push({ id: Date.now(), text: this.newTodo });
        this.newTodo = '';
      }
    },
    removeTodo(id) {
      this.todos = this.todos.filter(t => t.id !== id);
    },
  },
};
</script>

<style scoped>
.todo-app {
  max-width: 500px;
  margin: 0 auto;
}
input {
  width: 100%;
  padding: 8px;
}
li {
  display: flex;
  justify-content: space-between;
  padding: 8px 0;
}
</style>
`
	newContent := `<template>
  <div class="todo-app">
    <h1>{{ title }}</h1>
    <div class="input-group">
      <input v-model="newTodo" @keyup.enter="addTodo" placeholder="What needs to be done?">
      <button @click="addTodo" :disabled="!newTodo.trim()">Add</button>
    </div>
    <div class="filters">
      <button v-for="f in filters" :key="f" @click="filter = f" :class="{ active: filter === f }">
        {{ f }}
      </button>
    </div>
    <ul>
      <li v-for="todo in filteredTodos" :key="todo.id" :class="{ done: todo.done }">
        <input type="checkbox" v-model="todo.done">
        <span>{{ todo.text }}</span>
        <button @click="removeTodo(todo.id)">Delete</button>
      </li>
    </ul>
    <p class="stats">{{ remaining }} items left</p>
  </div>
</template>

<script>
export default {
  data() {
    return {
      title: 'Todo List',
      newTodo: '',
      todos: [],
      filter: 'All',
      filters: ['All', 'Active', 'Done'],
    };
  },
  computed: {
    filteredTodos() {
      if (this.filter === 'Active') return this.todos.filter(t => !t.done);
      if (this.filter === 'Done') return this.todos.filter(t => t.done);
      return this.todos;
    },
    remaining() {
      return this.todos.filter(t => !t.done).length;
    },
  },
  methods: {
    addTodo() {
      const text = this.newTodo.trim();
      if (text) {
        this.todos.push({ id: Date.now(), text, done: false });
        this.newTodo = '';
      }
    },
    removeTodo(id) {
      this.todos = this.todos.filter(t => t.id !== id);
    },
  },
};
</script>

<style scoped>
.todo-app {
  max-width: 600px;
  margin: 0 auto;
  padding: 20px;
}
.input-group {
  display: flex;
  gap: 8px;
}
input[type="text"] {
  flex: 1;
  padding: 10px;
  border: 1px solid #ddd;
  border-radius: 4px;
}
.filters {
  display: flex;
  gap: 4px;
  margin: 12px 0;
}
.filters button.active {
  font-weight: bold;
  text-decoration: underline;
}
li {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
  border-bottom: 1px solid #eee;
}
li.done span {
  text-decoration: line-through;
  opacity: 0.6;
}
.stats {
  color: #888;
  font-size: 14px;
}
</style>
`
	roundTripTest(t, oldContent, newContent)
}

// ============================================================
// 5. 多文件往返测试
// ============================================================

func TestMultiFilePythonAndHTML(t *testing.T) {
	files := map[string]string{
		"app.py":               "from flask import Flask, render_template\n\napp = Flask(__name__)\n\n@app.route('/')\ndef index():\n    return render_template('index.html', title='Home')\n\n@app.route('/about')\ndef about():\n    return render_template('about.html', title='About')\n",
		"templates/index.html": "<h1>{{ title }}</h1>\n<p>Welcome to our site.</p>\n<a href=\"/about\">About</a>\n",
	}

	changes := []FileChange{
		{
			Path:       "app.py",
			OldContent: files["app.py"],
			NewContent: "from flask import Flask, render_template, jsonify\n\napp = Flask(__name__)\n\n@app.route('/')\ndef index():\n    return render_template('index.html', title='Home', version='2.0')\n\n@app.route('/about')\ndef about():\n    return render_template('about.html', title='About')\n\n@app.route('/api/health')\ndef health():\n    return jsonify({'status': 'ok'})\n",
		},
		{
			Path:       "templates/index.html",
			OldContent: files["templates/index.html"],
			NewContent: "<h1>{{ title }}</h1>\n<p>Welcome to our site. Version {{ version }}.</p>\n<nav>\n    <a href=\"/about\">About</a>\n    <a href=\"/api/health\">Health</a>\n</nav>\n",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchStr := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchStr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(parsed, accessor)

	if result.Failed != 0 {
		for _, r := range result.Results {
			if !r.Success {
				t.Errorf("file %s failed: %s", r.Path, r.Error)
			}
		}
		t.FailNow()
	}

	if !strings.Contains(accessor.files["app.py"], "jsonify") {
		t.Error("app.py should contain jsonify import")
	}
	if !strings.Contains(accessor.files["templates/index.html"], "Version {{ version }}") {
		t.Error("index.html should contain version variable")
	}
}

func TestRoundtrip_ThreeFiles_WithNewAndDelete(t *testing.T) {
	changes := []FileChange{
		{
			Path:       "modify.go",
			OldContent: "package main\n\nvar x = 1\n",
			NewContent: "package main\n\nvar x = 42\n",
		},
		{
			Path:       "new.go",
			OldContent: "",
			NewContent: "package main\n\nfunc newFunc() {}\n",
		},
		{
			Path:       "delete.go",
			OldContent: "package main\n\nfunc old() {}\n",
			NewContent: "",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchText := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	accessor := &memAccessor{files: map[string]string{
		"modify.go": changes[0].OldContent,
		"delete.go": changes[2].OldContent,
	}}
	result := ApplyMultiFileDiff(parsed, accessor)
	if result.Failed != 0 {
		t.Errorf("roundtrip had %d failures", result.Failed)
		for _, r := range result.Results {
			if !r.Success {
				t.Logf("  %s: %s", r.Path, r.Error)
			}
		}
	}

	if accessor.files["modify.go"] != changes[0].NewContent {
		t.Errorf("modify.go: got %q, want %q", accessor.files["modify.go"], changes[0].NewContent)
	}
	if _, ok := accessor.files["new.go"]; !ok {
		t.Error("new.go should have been created")
	}
	if _, ok := accessor.files["delete.go"]; ok {
		t.Error("delete.go should have been deleted")
	}
}

// ============================================================
// 6. 端到端（E2E）场景测试
// ============================================================

func TestE2E_MultiLanguage_ProjectPatch(t *testing.T) {
	files := map[string]string{
		"index.html": "<!DOCTYPE html>\n<html>\n<head>\n    <title>App</title>\n    <link rel=\"stylesheet\" href=\"style.css\">\n</head>\n<body>\n    <div id=\"root\"></div>\n    <script src=\"app.js\"></script>\n</body>\n</html>\n",
		"style.css":  ".root {\n    display: flex;\n    flex-direction: column;\n    min-height: 100vh;\n}\n\n.header {\n    padding: 16px;\n    background: #333;\n    color: white;\n}\n\n.main {\n    flex: 1;\n    padding: 24px;\n}\n",
		"app.js":     "const app = {\n    init() {\n        console.log('App started');\n        this.render();\n    },\n    \n    render() {\n        const root = document.getElementById('root');\n        root.innerHTML = '<h1>Hello</h1>';\n    },\n    \n    destroy() {\n        console.log('App destroyed');\n    }\n};\n\ndocument.addEventListener('DOMContentLoaded', () => app.init());\n",
	}

	changes := []FileChange{
		{
			Path:       "index.html",
			OldContent: files["index.html"],
			NewContent: strings.Replace(files["index.html"], "<title>App</title>", "<title>My Dashboard</title>", 1),
		},
		{
			Path:       "style.css",
			OldContent: files["style.css"],
			NewContent: strings.Replace(files["style.css"], "background: #333;", "background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);", 1),
		},
		{
			Path:       "app.js",
			OldContent: files["app.js"],
			NewContent: strings.Replace(files["app.js"],
				"root.innerHTML = '<h1>Hello</h1>';",
				"root.innerHTML = '<h1>Dashboard</h1><p>Welcome back!</p>';", 1),
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchText := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(parsed.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(parsed.Files))
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(parsed, accessor)

	if result.Failed != 0 {
		t.Errorf("expected 0 failures, got %d", result.Failed)
	}
	for _, c := range changes {
		if accessor.files[c.Path] != c.NewContent {
			t.Errorf("%s: content mismatch", c.Path)
		}
	}
}

func TestE2E_PythonProjectRefactor(t *testing.T) {
	files := map[string]string{
		"main.py":   "from flask import Flask\nfrom routes import register_routes\n\napp = Flask(__name__)\nregister_routes(app)\n\nif __name__ == '__main__':\n    app.run(debug=True, port=5000)\n",
		"routes.py": "from flask import jsonify, request\n\ndef register_routes(app):\n    @app.route('/')\n    def index():\n        return jsonify({\"message\": \"Hello\"})\n    \n    @app.route('/users')\n    def users():\n        return jsonify({\"users\": []})\n",
		"config.py": "class Config:\n    DEBUG = True\n    PORT = 5000\n    DATABASE_URL = \"sqlite:///app.db\"\n",
	}

	changes := []FileChange{
		{
			Path:       "main.py",
			OldContent: files["main.py"],
			NewContent: "from flask import Flask\nfrom config import Config\nfrom routes import register_routes\n\napp = Flask(__name__)\napp.config.from_object(Config)\nregister_routes(app)\n\nif __name__ == '__main__':\n    app.run(debug=Config.DEBUG, port=Config.PORT)\n",
		},
		{
			Path:       "routes.py",
			OldContent: files["routes.py"],
			NewContent: "from flask import jsonify, request\n\ndef register_routes(app):\n    @app.route('/')\n    def index():\n        return jsonify({\"message\": \"Welcome to API v2\"})\n    \n    @app.route('/users')\n    def users():\n        return jsonify({\"users\": []})\n    \n    @app.route('/health')\n    def health():\n        return jsonify({\"status\": \"ok\"})\n",
		},
		{
			Path:       "config.py",
			OldContent: files["config.py"],
			NewContent: "import os\n\nclass Config:\n    DEBUG = os.getenv('DEBUG', 'true').lower() == 'true'\n    PORT = int(os.getenv('PORT', '5000'))\n    DATABASE_URL = os.getenv('DATABASE_URL', \"sqlite:///app.db\")\n    SECRET_KEY = os.getenv('SECRET_KEY', 'dev-secret')\n",
		},
	}

	mfd := DiffMultiFiles(changes, 3)
	patchText := FormatMultiFileDiff(mfd)

	parsed, err := ParseMultiFileDiff(patchText)
	if err != nil {
		t.Fatalf("parse error: %v", err)
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

	for _, c := range changes {
		if accessor.files[c.Path] != c.NewContent {
			t.Errorf("%s: content mismatch\n  got:  %q\n  want: %q", c.Path, accessor.files[c.Path], c.NewContent)
		}
	}
}

func TestE2E_LLMStylePatch_NoSpacePrefix(t *testing.T) {
	files := map[string]string{
		"a.go": "package a\n\nvar x = 1\n\nfunc main() {}\n",
		"b.go": "package b\n\nvar y = \"hello\"\n\nfunc helper() {}\n",
	}

	patch := "--- a.go\n+++ a.go\n@@ -1,5 +1,5 @@\npackage a\n\n-var x = 1\n+var x = 42\n\nfunc main() {}\n--- b.go\n+++ b.go\n@@ -1,5 +1,5 @@\npackage b\n\n-var y = \"hello\"\n+var y = \"world\"\n\nfunc helper() {}\n"

	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(mfd.Files))
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Failed != 0 {
		t.Errorf("LLM-style patch should succeed, got %d failures", result.Failed)
	}
	if !strings.Contains(accessor.files["a.go"], "var x = 42") {
		t.Error("a.go not modified")
	}
	if !strings.Contains(accessor.files["b.go"], `var y = "world"`) {
		t.Error("b.go not modified")
	}
}

func TestE2E_LLMStylePatch_TabPrefix(t *testing.T) {
	files := map[string]string{
		"a.go": "\tpackage a\n\tvar x = 1\n\tfunc main() {}\n",
		"b.go": "\tpackage b\n\tvar y = 1\n\tfunc helper() {}\n",
	}

	patch := "--- a.go\n+++ a.go\n@@ -1,3 +1,3 @@\n\tpackage a\n-\tvar x = 1\n+\tvar x = 2\n\tfunc main() {}\n--- b.go\n+++ b.go\n@@ -1,3 +1,3 @@\n\tpackage b\n-\tvar y = 1\n+\tvar y = 2\n\tfunc helper() {}\n"

	mfd, err := ParseMultiFileDiff(patch)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(mfd.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(mfd.Files))
	}

	accessor := &memAccessor{files: files}
	result := ApplyMultiFileDiff(mfd, accessor)

	if result.Failed != 0 {
		t.Errorf("tab-prefix patch should succeed, got %d failures", result.Failed)
	}
}
