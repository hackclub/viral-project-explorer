<script>
	import { onMount } from 'svelte';

	const API_KEY_STORAGE_KEY = 'api_key';

	let apiKey = '';
	let inputValue = '';
	let showKey = false;
	let isEditing = false;

	onMount(() => {
		const stored = localStorage.getItem(API_KEY_STORAGE_KEY);
		if (stored) {
			apiKey = stored;
		}
	});

	function saveApiKey() {
		if (inputValue.trim()) {
			apiKey = inputValue.trim();
			localStorage.setItem(API_KEY_STORAGE_KEY, apiKey);
			inputValue = '';
			isEditing = false;
		}
	}

	function clearApiKey() {
		apiKey = '';
		localStorage.removeItem(API_KEY_STORAGE_KEY);
		showKey = false;
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
</main>

<style>
	main {
		text-align: center;
		padding: 2em;
		max-width: 500px;
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

	@media (prefers-color-scheme: light) {
		.api-key-section {
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
	}
</style>




