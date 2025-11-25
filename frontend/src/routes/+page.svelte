<script>
	import { onMount } from 'svelte';
	import { 
		db, 
		dbReady, 
		isLoading, 
		loadError, 
		lastLoaded,
		loadDatabase, 
		loadFromCache,
		queryAll 
	} from '$lib/db';

	const API_KEY_STORAGE_KEY = 'api_key';

	let apiKey = '';
	let inputValue = '';
	let showKey = false;
	let isEditing = false;

	// Database display state
	let approvedProjects = [];
	let projectMentions = [];

	onMount(async () => {
		const stored = localStorage.getItem(API_KEY_STORAGE_KEY);
		if (stored) {
			apiKey = stored;
			// Try to load from cache first
			const loadedFromCache = await loadFromCache();
			if (loadedFromCache) {
				loadTableData();
			}
		}
	});

	function saveApiKey() {
		if (inputValue.trim()) {
			apiKey = inputValue.trim();
			localStorage.setItem(API_KEY_STORAGE_KEY, apiKey);
			inputValue = '';
			isEditing = false;
			// Clear previous data when API key changes
			approvedProjects = [];
			projectMentions = [];
		}
	}

	function clearApiKey() {
		apiKey = '';
		localStorage.removeItem(API_KEY_STORAGE_KEY);
		showKey = false;
		// Clear database data
		approvedProjects = [];
		projectMentions = [];
	}

	function startEditing() {
		isEditing = true;
		inputValue = '';
	}

	function cancelEditing() {
		isEditing = false;
		inputValue = '';
	}

	function getMaskedKey(key) {
		if (key.length <= 8) {
			return '•'.repeat(key.length);
		}
		return key.slice(0, 4) + '•'.repeat(key.length - 8) + key.slice(-4);
	}

	function handleKeydown(event) {
		if (event.key === 'Enter') {
			saveApiKey();
		} else if (event.key === 'Escape') {
			cancelEditing();
		}
	}

	async function handleLoadDatabase() {
		const success = await loadDatabase(apiKey);
		if (success) {
			loadTableData();
		}
	}

	function loadTableData() {
		// Query first 10 rows from approved_projects
		approvedProjects = queryAll('SELECT * FROM approved_projects LIMIT 10');
		
		// Query first 10 rows from ysws_project_mentions
		projectMentions = queryAll('SELECT * FROM ysws_project_mentions LIMIT 10');
	}

	function formatLastLoaded(date) {
		if (!date) return 'Never';
		const now = new Date();
		const diff = now - date;
		
		// Less than a minute ago
		if (diff < 60000) {
			return 'Just now';
		}
		
		// Less than an hour ago
		if (diff < 3600000) {
			const mins = Math.floor(diff / 60000);
			return `${mins} minute${mins === 1 ? '' : 's'} ago`;
		}
		
		// Less than a day ago
		if (diff < 86400000) {
			const hours = Math.floor(diff / 3600000);
			return `${hours} hour${hours === 1 ? '' : 's'} ago`;
		}
		
		// Show full date/time
		return date.toLocaleString();
	}
</script>

<main>
	<h1>Viral Project Explorer</h1>
	
	<div class="api-key-section">
		<h2>API Key</h2>
		
		{#if !apiKey && !isEditing}
			<p class="hint">Enter your API key to get started</p>
			<div class="input-group">
				<input
					type="password"
					bind:value={inputValue}
					placeholder="Enter your API key"
					on:keydown={handleKeydown}
				/>
				<button class="primary" on:click={saveApiKey} disabled={!inputValue.trim()}>
					Save
				</button>
			</div>
		{:else if isEditing}
			<p class="hint">Enter a new API key</p>
			<div class="input-group">
				<input
					type="password"
					bind:value={inputValue}
					placeholder="Enter new API key"
					on:keydown={handleKeydown}
				/>
				<button class="primary" on:click={saveApiKey} disabled={!inputValue.trim()}>
					Save
				</button>
				<button class="secondary" on:click={cancelEditing}>
					Cancel
				</button>
			</div>
		{:else}
			<div class="key-display">
				<span class="key-value">
					{showKey ? apiKey : getMaskedKey(apiKey)}
				</span>
				<button class="icon-btn" on:click={() => showKey = !showKey} title={showKey ? 'Hide' : 'Show'}>
					{showKey ? '🙈' : '👁️'}
				</button>
			</div>
			<div class="actions">
				<button class="secondary" on:click={startEditing}>
					Change Key
				</button>
				<button class="danger" on:click={clearApiKey}>
					Remove Key
				</button>
			</div>
			<p class="status">✓ API key configured</p>
		{/if}
	</div>

	{#if apiKey && !isEditing}
		<div class="database-section">
			<div class="database-header">
				<div class="database-status">
					{#if $dbReady}
						<span class="status-indicator ready">●</span>
						<span>Database loaded</span>
					{:else}
						<span class="status-indicator not-ready">●</span>
						<span>Database not loaded</span>
					{/if}
				</div>
				
				<div class="database-actions">
					<button 
						class="primary reload-btn" 
						on:click={handleLoadDatabase} 
						disabled={$isLoading}
					>
						{#if $isLoading}
							<span class="spinner"></span>
							Loading...
						{:else}
							🔄 {$dbReady ? 'Reload' : 'Load'} Database
						{/if}
					</button>
				</div>
				
				{#if $lastLoaded}
					<div class="last-loaded">
						Last loaded: <strong>{formatLastLoaded($lastLoaded)}</strong>
					</div>
				{/if}
			</div>

			{#if $loadError}
				<div class="error-message">
					{$loadError}
				</div>
			{/if}

			{#if approvedProjects.length > 0}
				<div class="table-container">
					<h2>Approved Projects (First 10)</h2>
					<div class="table-wrapper">
						<table>
							<thead>
								<tr>
									{#each Object.keys(approvedProjects[0]) as column}
										<th>{column}</th>
									{/each}
								</tr>
							</thead>
							<tbody>
								{#each approvedProjects as row}
									<tr>
										{#each Object.values(row) as value}
											<td>
												{#if value && (String(value).startsWith('http://') || String(value).startsWith('https://'))}
													<a href={value} target="_blank" rel="noopener noreferrer">
														{String(value).length > 40 ? String(value).slice(0, 40) + '...' : value}
													</a>
												{:else}
													{value ?? '—'}
												{/if}
											</td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}

			{#if projectMentions.length > 0}
				<div class="table-container">
					<h2>Project Mentions (First 10)</h2>
					<div class="table-wrapper">
						<table>
							<thead>
								<tr>
									{#each Object.keys(projectMentions[0]) as column}
										<th>{column}</th>
									{/each}
								</tr>
							</thead>
							<tbody>
								{#each projectMentions as row}
									<tr>
										{#each Object.values(row) as value}
											<td>
												{#if value && (String(value).startsWith('http://') || String(value).startsWith('https://'))}
													<a href={value} target="_blank" rel="noopener noreferrer">
														{String(value).length > 40 ? String(value).slice(0, 40) + '...' : value}
													</a>
												{:else}
													{value ?? '—'}
												{/if}
											</td>
										{/each}
									</tr>
								{/each}
							</tbody>
						</table>
					</div>
				</div>
			{/if}
		</div>
	{/if}
</main>

<style>
	main {
		text-align: center;
		padding: 2em;
		max-width: 100%;
		margin: 0 auto;
	}

	h1 {
		color: #ff3e00;
		font-size: 2.5em;
		font-weight: 600;
		margin-bottom: 0.5em;
	}

	h2 {
		font-size: 1.2em;
		font-weight: 500;
		margin-bottom: 0.5em;
		color: inherit;
	}

	.api-key-section {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 12px;
		padding: 1.5em;
		margin-top: 2em;
		max-width: 500px;
		margin-left: auto;
		margin-right: auto;
	}

	.hint {
		color: rgba(255, 255, 255, 0.6);
		font-size: 0.9em;
		margin-bottom: 1em;
	}

	.input-group {
		display: flex;
		gap: 0.5em;
		justify-content: center;
		flex-wrap: wrap;
	}

	input {
		padding: 0.6em 1em;
		border-radius: 8px;
		border: 1px solid rgba(255, 255, 255, 0.2);
		background: rgba(0, 0, 0, 0.3);
		color: inherit;
		font-size: 1em;
		min-width: 200px;
		flex: 1;
	}

	input:focus {
		outline: none;
		border-color: #ff3e00;
	}

	input::placeholder {
		color: rgba(255, 255, 255, 0.4);
	}

	button {
		padding: 0.6em 1.2em;
		border-radius: 8px;
		border: none;
		font-size: 0.9em;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s ease;
		display: inline-flex;
		align-items: center;
		gap: 0.5em;
	}

	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	button.primary {
		background: #ff3e00;
		color: white;
	}

	button.primary:hover:not(:disabled) {
		background: #ff5722;
	}

	button.secondary {
		background: rgba(255, 255, 255, 0.1);
		color: inherit;
		border: 1px solid rgba(255, 255, 255, 0.2);
	}

	button.secondary:hover {
		background: rgba(255, 255, 255, 0.15);
	}

	button.danger {
		background: rgba(220, 53, 69, 0.2);
		color: #ff6b7a;
		border: 1px solid rgba(220, 53, 69, 0.3);
	}

	button.danger:hover {
		background: rgba(220, 53, 69, 0.3);
	}

	button.icon-btn {
		background: transparent;
		padding: 0.4em;
		font-size: 1.1em;
		border: none;
	}

	button.icon-btn:hover {
		opacity: 0.8;
	}

	.key-display {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5em;
		margin-bottom: 1em;
		background: rgba(0, 0, 0, 0.2);
		padding: 0.8em 1em;
		border-radius: 8px;
		font-family: monospace;
	}

	.key-value {
		word-break: break-all;
	}

	.actions {
		display: flex;
		gap: 0.5em;
		justify-content: center;
		flex-wrap: wrap;
	}

	.status {
		color: #4caf50;
		font-size: 0.85em;
		margin-top: 1em;
		margin-bottom: 0;
	}

	/* Database section styles */
	.database-section {
		margin-top: 2em;
	}

	.database-header {
		background: rgba(255, 255, 255, 0.05);
		border: 1px solid rgba(255, 255, 255, 0.1);
		border-radius: 12px;
		padding: 1.5em;
		margin-bottom: 2em;
		max-width: 600px;
		margin-left: auto;
		margin-right: auto;
		display: flex;
		flex-direction: column;
		gap: 1em;
		align-items: center;
	}

	.database-status {
		display: flex;
		align-items: center;
		gap: 0.5em;
		font-size: 1.1em;
	}

	.status-indicator {
		font-size: 1.2em;
	}

	.status-indicator.ready {
		color: #4caf50;
	}

	.status-indicator.not-ready {
		color: #ff9800;
	}

	.database-actions {
		display: flex;
		gap: 0.5em;
	}

	.reload-btn {
		padding: 0.8em 1.5em;
		font-size: 1em;
	}

	.last-loaded {
		font-size: 0.9em;
		color: rgba(255, 255, 255, 0.6);
	}

	.last-loaded strong {
		color: rgba(255, 255, 255, 0.9);
	}

	.spinner {
		width: 16px;
		height: 16px;
		border: 2px solid rgba(255, 255, 255, 0.3);
		border-top-color: white;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}

	.error-message {
		background: rgba(220, 53, 69, 0.15);
		border: 1px solid rgba(220, 53, 69, 0.3);
		color: #ff6b7a;
		padding: 1em;
		border-radius: 8px;
		margin-top: 1em;
		max-width: 500px;
		margin-left: auto;
		margin-right: auto;
	}

	.table-container {
		margin-top: 2em;
		text-align: left;
	}

	.table-container h2 {
		text-align: center;
		margin-bottom: 1em;
		color: #ff3e00;
	}

	.table-wrapper {
		overflow-x: auto;
		border-radius: 8px;
		border: 1px solid rgba(255, 255, 255, 0.1);
	}

	table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.85em;
	}

	th, td {
		padding: 0.75em 1em;
		text-align: left;
		border-bottom: 1px solid rgba(255, 255, 255, 0.1);
		white-space: nowrap;
		max-width: 300px;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	th {
		background: rgba(255, 255, 255, 0.05);
		font-weight: 600;
		position: sticky;
		top: 0;
		color: #ff3e00;
	}

	tr:hover {
		background: rgba(255, 255, 255, 0.02);
	}

	td a {
		color: #646cff;
	}

	td a:hover {
		color: #535bf2;
	}

	@media (prefers-color-scheme: light) {
		.api-key-section,
		.database-header {
			background: rgba(0, 0, 0, 0.03);
			border-color: rgba(0, 0, 0, 0.1);
		}

		.hint {
			color: rgba(0, 0, 0, 0.6);
		}

		input {
			background: white;
			border-color: rgba(0, 0, 0, 0.2);
		}

		input::placeholder {
			color: rgba(0, 0, 0, 0.4);
		}

		button.secondary {
			background: rgba(0, 0, 0, 0.05);
			border-color: rgba(0, 0, 0, 0.2);
		}

		button.secondary:hover {
			background: rgba(0, 0, 0, 0.1);
		}

		.key-display {
			background: rgba(0, 0, 0, 0.05);
		}

		.table-wrapper {
			border-color: rgba(0, 0, 0, 0.1);
		}

		th, td {
			border-bottom-color: rgba(0, 0, 0, 0.1);
		}

		th {
			background: rgba(0, 0, 0, 0.03);
		}

		tr:hover {
			background: rgba(0, 0, 0, 0.02);
		}

		.last-loaded {
			color: rgba(0, 0, 0, 0.6);
		}

		.last-loaded strong {
			color: rgba(0, 0, 0, 0.9);
		}

		.status-indicator.not-ready {
			color: #ed6c02;
		}

		.spinner {
			border-color: rgba(255, 255, 255, 0.3);
			border-top-color: white;
		}
	}
</style>
