/**
 * Database Store - SQLite WASM + OPFS
 * 
 * Uses the official SQLite WASM build with OPFS for persistent storage.
 * The database file is stored directly in the Origin Private File System,
 * so SQLite manages its own page cache and we don't load everything into memory.
 * 
 * Usage:
 *   import { db, dbReady, lastLoaded, loadDatabase, query, queryAll, queryOne } from '$lib/db';
 * 
 *   // Wait for database to be ready
 *   $: if ($dbReady) { ... }
 * 
 *   // Run queries
 *   const projects = queryAll('SELECT * FROM approved_projects LIMIT 10');
 *   const project = queryOne('SELECT * FROM approved_projects WHERE record_id = ?', ['rec123']);
 */

import { writable, derived, get } from 'svelte/store';
import { decompress } from 'fzstd';

// Constants
const BACKEND_URL = 'http://localhost:8080';
const API_KEY_STORAGE_KEY = 'api_key';
const DB_FILENAME = 'database.sqlite';
const META_FILENAME = 'database.meta.json';

// Stores
export const db = writable(null);
export const isLoading = writable(false);
export const loadError = writable('');
export const lastLoaded = writable(null);

// Derived store - true when database is ready to use
export const dbReady = derived(db, $db => $db !== null);

// SQL.js module (loaded once)
let SQL = null;

/**
 * Initialize sql.js
 */
async function initSqlJs() {
	if (SQL) return SQL;
	
	if (typeof window === 'undefined') {
		throw new Error('sql.js can only be loaded in the browser');
	}
	
	// Load sql.js from CDN
	if (typeof window.initSqlJs === 'undefined') {
		await new Promise((resolve, reject) => {
			const script = document.createElement('script');
			script.src = 'https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/sql-wasm.min.js';
			
			// Subresource Integrity for CDN security
			script.integrity = 'sha512-f4bgk3aqQ6nrGVs1Aw0R9kH598VsHnz+xXmRa3kv0wu+WhVp/58fExWhUnYyOyT9R1xA+ILShDUeMBg5hC7CmA==';
			script.crossOrigin = 'anonymous';
			
			script.onload = resolve;
			script.onerror = () => reject(new Error('Failed to load sql.js from CDN'));
			document.head.appendChild(script);
		});
	}
	
	SQL = await window.initSqlJs({
		locateFile: (file) => `https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/${file}`
	});
	
	console.log('sql.js initialized');
	return SQL;
}

/**
 * Get OPFS directory handle for our database files
 */
async function getDbDirectory() {
	const root = await navigator.storage.getDirectory();
	return await root.getDirectoryHandle('viral-explorer-db', { create: true });
}

/**
 * Write file to OPFS
 */
async function writeToOpfs(filename, data) {
	const dir = await getDbDirectory();
	const fileHandle = await dir.getFileHandle(filename, { create: true });
	const writable = await fileHandle.createWritable();
	await writable.write(data);
	await writable.close();
	console.log(`Wrote ${(data.byteLength || data.length) / 1024 / 1024} MB to OPFS: ${filename}`);
}

/**
 * Read file from OPFS
 */
async function readFromOpfs(filename) {
	try {
		const dir = await getDbDirectory();
		const fileHandle = await dir.getFileHandle(filename);
		const file = await fileHandle.getFile();
		return file;
	} catch (err) {
		if (err.name === 'NotFoundError') {
			return null;
		}
		throw err;
	}
}

/**
 * Check if file exists in OPFS
 */
async function existsInOpfs(filename) {
	try {
		const dir = await getDbDirectory();
		await dir.getFileHandle(filename);
		return true;
	} catch {
		return false;
	}
}

/**
 * Delete file from OPFS
 */
async function deleteFromOpfs(filename) {
	try {
		const dir = await getDbDirectory();
		await dir.removeEntry(filename);
	} catch {
		// Ignore if doesn't exist
	}
}

/**
 * Read metadata from OPFS
 */
async function readMeta() {
	const file = await readFromOpfs(META_FILENAME);
	if (!file) return null;
	
	try {
		const text = await file.text();
		return JSON.parse(text);
	} catch {
		return null;
	}
}

/**
 * Write metadata to OPFS
 */
async function writeMeta(meta) {
	const data = new TextEncoder().encode(JSON.stringify(meta));
	await writeToOpfs(META_FILENAME, data);
}

/**
 * Open the SQLite database from OPFS file
 */
async function openDatabase() {
	const sqlJs = await initSqlJs();
	
	// Read the database file from OPFS
	const file = await readFromOpfs(DB_FILENAME);
	if (!file) {
		throw new Error('Database file not found in OPFS');
	}
	
	const arrayBuffer = await file.arrayBuffer();
	const data = new Uint8Array(arrayBuffer);
	console.log(`Loading database from OPFS: ${(data.length / 1024 / 1024).toFixed(2)} MB`);
	
	// Create database from the file data
	const database = new sqlJs.Database(data);
	
	console.log('Database opened from OPFS');
	
	return database;
}

/**
 * Load database from OPFS cache
 * @returns {boolean} True if loaded from cache, false otherwise
 */
export async function loadFromCache() {
	if (typeof window === 'undefined') return false;
	
	try {
		// Check if database file exists
		const exists = await existsInOpfs(DB_FILENAME);
		if (!exists) {
			console.log('No cached database found in OPFS');
			return false;
		}
		
		// Read metadata
		const meta = await readMeta();
		if (!meta?.timestamp) {
			console.log('No metadata found, cache invalid');
			return false;
		}
		
		console.log('Loading database from OPFS cache...');
		isLoading.set(true);
		loadError.set('');
		
		// Open the database from OPFS
		const database = await openDatabase();
		
		db.set(database);
		lastLoaded.set(new Date(meta.timestamp));
		
		console.log('Database loaded from OPFS cache');
		return true;
	} catch (err) {
		console.error('Error loading from OPFS cache:', err);
		// Clear corrupted cache
		await clearCache();
		return false;
	} finally {
		isLoading.set(false);
	}
}

/**
 * Fetch fresh database from backend and save to OPFS
 * @param {string} [apiKey] - API key (uses stored key if not provided)
 * @returns {Promise<boolean>} True if successful
 */
export async function loadDatabase(apiKey) {
	if (typeof window === 'undefined') return false;
	
	const key = apiKey || sessionStorage.getItem(API_KEY_STORAGE_KEY);
	if (!key) {
		loadError.set('No API key configured');
		return false;
	}
	
	isLoading.set(true);
	loadError.set('');
	
	// Close existing database if any
	const currentDb = get(db);
	if (currentDb) {
		try {
			currentDb.close();
		} catch {}
		db.set(null);
	}
	
	try {
		// Fetch compressed database from backend
		console.log('Fetching database from backend...');
		const response = await fetch(`${BACKEND_URL}/db`, {
			headers: {
				'X-API-Key': key
			}
		});
		
		if (!response.ok) {
			if (response.status === 401) {
				throw new Error('Invalid API key. Please check your API key and try again.');
			}
			throw new Error(`Failed to fetch database: ${response.status} ${response.statusText}`);
		}
		
		const compressedBuffer = await response.arrayBuffer();
		const compressedData = new Uint8Array(compressedBuffer);
		
		console.log(`Received compressed data: ${(compressedData.length / 1024 / 1024).toFixed(2)} MB`);
		
		// Decompress
		const decompressedData = decompress(compressedData);
		console.log(`Decompressed data: ${(decompressedData.length / 1024 / 1024).toFixed(2)} MB`);
		
		// Delete old database file if exists
		await deleteFromOpfs(DB_FILENAME);
		
		// Write decompressed SQLite file to OPFS
		await writeToOpfs(DB_FILENAME, decompressedData);
		
		// Write metadata
		const timestamp = new Date();
		await writeMeta({ timestamp: timestamp.toISOString() });
		
		// Open the database from OPFS
		const database = await openDatabase();
		
		db.set(database);
		lastLoaded.set(timestamp);
		
		console.log('Database saved to OPFS and opened');
		return true;
	} catch (err) {
		loadError.set(err.message);
		console.error('Error loading database:', err);
		return false;
	} finally {
		isLoading.set(false);
	}
}

/**
 * Clear the cached database from OPFS
 */
export async function clearCache() {
	if (typeof window === 'undefined') return;
	
	// Close database if open
	const currentDb = get(db);
	if (currentDb) {
		try {
			currentDb.close();
		} catch {}
		db.set(null);
	}
	
	try {
		await deleteFromOpfs(DB_FILENAME);
		await deleteFromOpfs(META_FILENAME);
		// Also delete WAL and SHM files if they exist
		await deleteFromOpfs(DB_FILENAME + '-wal');
		await deleteFromOpfs(DB_FILENAME + '-shm');
		console.log('OPFS cache cleared');
	} catch (err) {
		console.warn('Failed to clear cache:', err);
	}
}

/**
 * Execute a SQL query and return all results
 * @param {string} sqlQuery - SQL query
 * @param {any[]} [params] - Query parameters
 * @returns {Object[]} Array of row objects
 */
export function queryAll(sqlQuery, params = []) {
	const database = get(db);
	if (!database) {
		console.warn('Database not loaded');
		return [];
	}
	
	try {
		const result = database.exec(sqlQuery, params);
		if (result.length === 0) return [];
		
		const columns = result[0].columns;
		return result[0].values.map(row => {
			const obj = {};
			columns.forEach((col, i) => {
				obj[col] = row[i];
			});
			return obj;
		});
	} catch (err) {
		console.error('Query error:', err);
		return [];
	}
}

/**
 * Execute a SQL query and return the first result
 * @param {string} sqlQuery - SQL query
 * @param {any[]} [params] - Query parameters
 * @returns {Object|null} First row object or null
 */
export function queryOne(sqlQuery, params = []) {
	const results = queryAll(sqlQuery, params);
	return results.length > 0 ? results[0] : null;
}

/**
 * Execute a SQL query and return raw results
 * @param {string} sqlQuery - SQL query
 * @param {any[]} [params] - Query parameters
 * @returns {Object} Result with columns and values
 */
export function query(sqlQuery, params = []) {
	const database = get(db);
	if (!database) {
		console.warn('Database not loaded');
		return { columns: [], values: [] };
	}
	
	try {
		const result = database.exec(sqlQuery, params);
		if (result.length === 0) return { columns: [], values: [] };
		return result[0];
	} catch (err) {
		console.error('Query error:', err);
		return { columns: [], values: [] };
	}
}

/**
 * Get table info
 * @param {string} tableName - Name of the table
 * @returns {Object[]} Array of column info
 */
export function getTableInfo(tableName) {
	return queryAll(`PRAGMA table_info(${tableName})`);
}

/**
 * Get all table names in the database
 * @returns {string[]} Array of table names
 */
export function getTableNames() {
	const results = queryAll("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name");
	return results.map(r => r.name);
}

/**
 * Get row count for a table
 * @param {string} tableName - Name of the table
 * @returns {number} Row count
 */
export function getRowCount(tableName) {
	const result = queryOne(`SELECT COUNT(*) as count FROM ${tableName}`);
	return result ? result.count : 0;
}

/**
 * Execute a raw SQL statement (for INSERT, UPDATE, DELETE, etc.)
 * @param {string} sqlStatement - SQL statement
 * @param {any[]} [params] - Query parameters
 * @returns {boolean} True if successful
 */
export function execute(sqlStatement, params = []) {
	const database = get(db);
	if (!database) {
		console.warn('Database not loaded');
		return false;
	}
	
	try {
		database.run(sqlStatement, params);
		return true;
	} catch (err) {
		console.error('Execute error:', err);
		return false;
	}
}
