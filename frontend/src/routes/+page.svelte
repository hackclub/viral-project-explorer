<script>
	import { onMount, tick } from 'svelte';
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

	// Tree state
	let codeUrls = [];
	let loadedCodeUrlsCount = 0;
	let totalCodeUrls = 0;
	let totalHours = 0;  // Sum of all hours for weighted projects calculation
	const CODE_URL_BATCH_SIZE = 50;
	let isLoadingMore = false;
	
	// Expanded state objects (using objects for better Svelte reactivity)
	let expandedCodeUrls = {};  // code_url -> { projects: [], articles: [], articleGroups: [], loaded: boolean }
	let expandedArticleGroups = {};  // url -> boolean (expanded state)
	let expandedProjects = {};  // record_id -> { articles: [], loaded: boolean }

	// Filter and sort state
	let selectedYsws = [];  // Array of selected YSWS names
	let yswsSearchQuery = '';  // Search input for YSWS autocomplete
	let showYswsDropdown = false;  // Whether to show autocomplete dropdown
	let sortBy = 'date';  // 'date', 'hours', or 'mentions'
	let minMentions = 0;  // Minimum number of article mentions to show
	let availableYsws = [];  // List of available YSWS names
	let yswsInputRef;  // Reference to the search input
	let selectedEmailHash = null;  // Filter by user email_hash
	let selectedUserName = '';  // Display name for the filtered user
	let selectedCountry = null;  // Filter by country

	// Scroll container ref
	let scrollContainer;

	onMount(async () => {
		const stored = localStorage.getItem(API_KEY_STORAGE_KEY);
		if (stored) {
			apiKey = stored;
			const loadedFromCache = await loadFromCache();
			if (loadedFromCache) {
				await loadInitialData();
			}
		}
	});

	async function loadInitialData() {
		// Load available YSWS names for filter dropdown
		const yswsResult = queryAll('SELECT DISTINCT ysws_name FROM approved_projects WHERE ysws_name IS NOT NULL AND ysws_name != "" ORDER BY ysws_name');
		availableYsws = yswsResult.map(r => r.ysws_name);
		
		// Get total count of unique code URLs (with filter applied)
		await refreshCodeUrls();
	}

	async function refreshCodeUrls() {
		// Reset state
		loadedCodeUrlsCount = 0;
		codeUrls = [];
		expandedCodeUrls = {};
		expandedArticleGroups = {};
		expandedProjects = {};
		
		// Build WHERE clause for filter
		let whereClause = '1=1';
		let params = [];
		if (selectedYsws.length > 0) {
			const placeholders = selectedYsws.map(() => '?').join(', ');
			whereClause += ` AND ap.ysws_name IN (${placeholders})`;
			params.push(...selectedYsws);
		}
		if (selectedEmailHash) {
			whereClause += ` AND ap.email_hash = ?`;
			params.push(selectedEmailHash);
		}
		if (selectedCountry) {
			whereClause += ` AND ap.geocoded_country = ?`;
			params.push(selectedCountry);
		}
		
		// Get total count with filter (including min mentions)
		let countQuery;
		if (minMentions > 0) {
			countQuery = queryAll(`
				SELECT COUNT(*) as count FROM (
					SELECT ap.code_url
					FROM approved_projects ap
					LEFT JOIN ysws_project_mentions m ON m.ysws_approved_project = ap.record_id
					WHERE ${whereClause}
					GROUP BY ap.code_url
					HAVING COUNT(DISTINCT m.url) >= ${minMentions}
				)
			`, params);
		} else {
			countQuery = queryAll(`SELECT COUNT(DISTINCT code_url) as count FROM approved_projects ap WHERE ${whereClause}`, params);
		}
		totalCodeUrls = countQuery[0]?.count || 0;
		
		// Get total hours for weighted projects calculation
		let hoursQuery;
		if (minMentions > 0) {
			hoursQuery = queryAll(`
				SELECT SUM(hours_spent) as total_hours FROM approved_projects ap
				WHERE ${whereClause} AND ap.code_url IN (
					SELECT ap2.code_url
					FROM approved_projects ap2
					LEFT JOIN ysws_project_mentions m ON m.ysws_approved_project = ap2.record_id
					GROUP BY ap2.code_url
					HAVING COUNT(DISTINCT m.url) >= ${minMentions}
				)
			`, params);
		} else {
			hoursQuery = queryAll(`SELECT SUM(hours_spent) as total_hours FROM approved_projects ap WHERE ${whereClause}`, params);
		}
		totalHours = hoursQuery[0]?.total_hours || 0;
		
		// Load first batch
		await loadMoreCodeUrls();
	}

	async function loadMoreCodeUrls() {
		if (isLoadingMore || loadedCodeUrlsCount >= totalCodeUrls) return;
		
		isLoadingMore = true;
		
		// Build WHERE clause for filter
		let whereClause = '1=1';
		let params = [];
		if (selectedYsws.length > 0) {
			const placeholders = selectedYsws.map(() => '?').join(', ');
			whereClause += ` AND ap.ysws_name IN (${placeholders})`;
			params.push(...selectedYsws);
		}
		if (selectedEmailHash) {
			whereClause += ` AND ap.email_hash = ?`;
			params.push(selectedEmailHash);
		}
		if (selectedCountry) {
			whereClause += ` AND ap.geocoded_country = ?`;
			params.push(selectedCountry);
		}
		
		// Build HAVING clause for min mentions filter
		let havingClause = minMentions > 0 ? `HAVING article_count >= ${minMentions}` : '';
		
		// Build ORDER BY clause based on sort option
		let orderBy;
		switch (sortBy) {
			case 'hours':
				orderBy = 'total_hours DESC, latest_approved_at DESC';
				break;
			case 'mentions':
				orderBy = 'article_count DESC, latest_approved_at DESC';
				break;
			default:
				orderBy = 'latest_approved_at DESC, code_url ASC';
		}
		
		const newUrls = queryAll(`
			SELECT 
				ap.code_url,
				COUNT(DISTINCT ap.record_id) as project_count,
				MAX(ap.approved_at) as latest_approved_at,
				SUM(ap.hours_spent) as total_hours,
				GROUP_CONCAT(DISTINCT ap.geocoded_country) as countries,
				GROUP_CONCAT(DISTINCT ap.ysws_name) as ysws_names,
				COUNT(DISTINCT m.url) as article_count
			FROM approved_projects ap
			LEFT JOIN ysws_project_mentions m ON m.ysws_approved_project = ap.record_id
			WHERE ${whereClause}
			GROUP BY ap.code_url 
			${havingClause}
			ORDER BY ${orderBy}
			LIMIT ? OFFSET ?
		`, [...params, CODE_URL_BATCH_SIZE, loadedCodeUrlsCount]);
		
		codeUrls = [...codeUrls, ...newUrls];
		loadedCodeUrlsCount += newUrls.length;
		isLoadingMore = false;
	}

	function toggleCodeUrl(codeUrl) {
		// Use empty string as key for null code_url
		const key = codeUrl ?? '__NULL__';
		
		if (expandedCodeUrls[key]) {
			delete expandedCodeUrls[key];
			expandedCodeUrls = { ...expandedCodeUrls };
		} else {
			// Load projects for this code URL, sorted by most recent approved_at
			let projects;
			if (codeUrl === null || codeUrl === '') {
				projects = queryAll(`
					SELECT record_id, first_name, last_name, git_hub_username, geocoded_country, playable_url, code_url,
						hours_spent, approved_at, override_hours_spent_justification, age_when_approved, ysws_name, email_hash
					FROM approved_projects 
					WHERE code_url IS NULL OR code_url = ''
					ORDER BY approved_at DESC
				`);
			} else {
				projects = queryAll(`
					SELECT record_id, first_name, last_name, git_hub_username, geocoded_country, playable_url, code_url,
						hours_spent, approved_at, override_hours_spent_justification, age_when_approved, ysws_name, email_hash
					FROM approved_projects 
					WHERE code_url = ?
					ORDER BY approved_at DESC
				`, [codeUrl]);
			}
			
			// Load all articles for all projects with this code_url
			const projectIds = projects.map(p => p.record_id);
			let articles = [];
			if (projectIds.length > 0) {
				const placeholders = projectIds.map(() => '?').join(', ');
				articles = queryAll(`
					SELECT 
						id, headline, source, url, link_found_at, 
						date, weighted_engagement_points, engagement_count, 
						engagement_type, mentions_hack_club, published_by_hack_club,
						ysws_approved_project
					FROM ysws_project_mentions 
					WHERE ysws_approved_project IN (${placeholders})
					ORDER BY date DESC, link_found_at DESC
				`, projectIds);
			}
			
			// Group articles by URL
			const articleGroups = groupArticlesByUrl(articles);
			
			expandedCodeUrls = { ...expandedCodeUrls, [key]: { projects, articles, articleGroups, loaded: true } };
		}
	}

	function groupArticlesByUrl(articles) {
		const groups = {};
		
		for (const article of articles) {
			const url = article.url || article.link_found_at || '(no URL)';
			if (!groups[url]) {
				groups[url] = {
					url,
					articles: [],
					latestDate: null,
					totalEngagement: 0,
					headline: article.headline,
					source: article.source
				};
			}
			groups[url].articles.push(article);
			
			// Track latest date for sorting
			const articleDate = article.date || article.link_found_at;
			if (articleDate && (!groups[url].latestDate || articleDate > groups[url].latestDate)) {
				groups[url].latestDate = articleDate;
				// Update headline to most recent
				groups[url].headline = article.headline || groups[url].headline;
				groups[url].source = article.source || groups[url].source;
			}
			
			// Sum engagement
			groups[url].totalEngagement += article.weighted_engagement_points || 0;
		}
		
		// Convert to array and sort by most recent date
		return Object.values(groups).sort((a, b) => {
			if (!a.latestDate && !b.latestDate) return 0;
			if (!a.latestDate) return 1;
			if (!b.latestDate) return -1;
			return b.latestDate.localeCompare(a.latestDate);
		});
	}

	function toggleArticleGroup(url) {
		if (expandedArticleGroups[url]) {
			delete expandedArticleGroups[url];
			expandedArticleGroups = { ...expandedArticleGroups };
		} else {
			expandedArticleGroups = { ...expandedArticleGroups, [url]: true };
		}
	}

	function toggleProject(recordId) {
		if (expandedProjects[recordId]) {
			delete expandedProjects[recordId];
			expandedProjects = { ...expandedProjects };
		} else {
			// Load articles for this project
			// Note: ysws_approved_project links to the project's record_id
			const articles = queryAll(`
				SELECT 
					id, headline, source, url, link_found_at, 
					date, weighted_engagement_points, engagement_count, 
					engagement_type, mentions_hack_club, published_by_hack_club
				FROM ysws_project_mentions 
				WHERE ysws_approved_project = ?
				ORDER BY weighted_engagement_points DESC, date DESC
			`, [recordId]);
			
			expandedProjects = { ...expandedProjects, [recordId]: { articles, loaded: true } };
		}
	}

	function handleScroll(e) {
		const { scrollTop, scrollHeight, clientHeight } = e.target;
		if (scrollHeight - scrollTop - clientHeight < 200) {
			loadMoreCodeUrls();
		}
	}

	function saveApiKey() {
		if (inputValue.trim()) {
			apiKey = inputValue.trim();
			localStorage.setItem(API_KEY_STORAGE_KEY, apiKey);
			inputValue = '';
			isEditing = false;
			codeUrls = [];
			expandedCodeUrls = {};
			expandedProjects = {};
		}
	}

	function clearApiKey() {
		apiKey = '';
		localStorage.removeItem(API_KEY_STORAGE_KEY);
		showKey = false;
		codeUrls = [];
		expandedCodeUrls = {};
		expandedProjects = {};
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
		if (key.length <= 8) return '•'.repeat(key.length);
		return key.slice(0, 4) + '•'.repeat(key.length - 8) + key.slice(-4);
	}

	function handleKeydown(event) {
		if (event.key === 'Enter') saveApiKey();
		else if (event.key === 'Escape') cancelEditing();
	}

	async function handleLoadDatabase() {
		const success = await loadDatabase(apiKey);
		if (success) {
			await loadInitialData();
		}
	}

	function formatLastLoaded(date) {
		if (!date) return 'Never';
		const now = new Date();
		const diff = now - date;
		if (diff < 60000) return 'Just now';
		if (diff < 3600000) {
			const mins = Math.floor(diff / 60000);
			return `${mins} minute${mins === 1 ? '' : 's'} ago`;
		}
		if (diff < 86400000) {
			const hours = Math.floor(diff / 3600000);
			return `${hours} hour${hours === 1 ? '' : 's'} ago`;
		}
		return date.toLocaleString();
	}

	function formatUrl(url) {
		if (!url) return '(no URL)';
		try {
			const parsed = new URL(url);
			return parsed.hostname + parsed.pathname;
		} catch {
			return url.length > 50 ? url.slice(0, 50) + '...' : url;
		}
	}

	function getEngagementBadge(points) {
		if (!points || points < 10) return null;
		if (points >= 1000) return { class: 'viral', label: '🔥 Viral' };
		if (points >= 100) return { class: 'hot', label: '🌟 Hot' };
		return { class: 'warm', label: '✨ Notable' };
	}

	function formatDate(dateStr) {
		if (!dateStr) return '';
		// Extract just the date part (YYYY-MM-DD) from ISO strings or date-time strings
		const match = dateStr.match(/^(\d{4}-\d{2}-\d{2})/);
		return match ? match[1] : dateStr;
	}

	function formatCountries(countriesStr) {
		if (!countriesStr) return '';
		// Split and deduplicate countries, filter out empty ones
		const countries = [...new Set(countriesStr.split(',').map(c => c.trim()).filter(Boolean))];
		if (countries.length === 0) return '';
		if (countries.length <= 3) return countries.join(', ');
		return `${countries.slice(0, 3).join(', ')} +${countries.length - 3}`;
	}

	function formatYswsNames(namesStr) {
		if (!namesStr) return [];
		// Split and deduplicate names, filter out empty ones
		return [...new Set(namesStr.split(',').map(n => n.trim()).filter(Boolean))];
	}

	function formatHours(hours) {
		const num = Number(hours);
		if (isNaN(num)) return hours;
		// If it's a whole number, show without decimal
		if (num % 1 === 0) return num.toString();
		// Otherwise round to 1 decimal place
		return num.toFixed(1);
	}

	// Computed filtered YSWS suggestions
	$: filteredYsws = availableYsws.filter(ysws => 
		!selectedYsws.includes(ysws) && 
		ysws.toLowerCase().includes(yswsSearchQuery.toLowerCase())
	);

	function addYsws(ysws) {
		if (!selectedYsws.includes(ysws)) {
			selectedYsws = [...selectedYsws, ysws];
			yswsSearchQuery = '';
			showYswsDropdown = false;
			refreshCodeUrls();
		}
	}

	function removeYsws(ysws) {
		selectedYsws = selectedYsws.filter(y => y !== ysws);
		refreshCodeUrls();
	}

	function handleYswsInputKeydown(event) {
		if (event.key === 'Enter' && filteredYsws.length > 0) {
			event.preventDefault();
			addYsws(filteredYsws[0]);
		} else if (event.key === 'Escape') {
			showYswsDropdown = false;
			yswsSearchQuery = '';
		} else if (event.key === 'Backspace' && yswsSearchQuery === '' && selectedYsws.length > 0) {
			// Remove last selected YSWS when backspace on empty input
			removeYsws(selectedYsws[selectedYsws.length - 1]);
		}
	}

	function handleYswsInputFocus() {
		showYswsDropdown = true;
	}

	function handleYswsInputBlur() {
		// Delay hiding to allow click on dropdown items
		setTimeout(() => {
			showYswsDropdown = false;
		}, 200);
	}

	function setUserFilter(emailHash, displayName) {
		if (emailHash) {
			selectedEmailHash = emailHash;
			selectedUserName = displayName || 'Unknown User';
			refreshCodeUrls();
		}
	}

	function clearUserFilter() {
		selectedEmailHash = null;
		selectedUserName = '';
		refreshCodeUrls();
	}

	function setCountryFilter(country) {
		if (country) {
			selectedCountry = country;
			refreshCodeUrls();
		}
	}

	function clearCountryFilter() {
		selectedCountry = null;
		refreshCodeUrls();
	}

	function addYswsFromBadge(ysws) {
		if (ysws && !selectedYsws.includes(ysws)) {
			selectedYsws = [...selectedYsws, ysws];
			refreshCodeUrls();
		}
	}
</script>

<main>
	<header>
		<h1>🔬 Viral Project Explorer</h1>
		<p class="subtitle">Explore projects and the articles that link to them</p>
	</header>
	
	<div class="api-key-section">
		{#if !apiKey && !isEditing}
			<div class="input-group">
				<input
					type="password"
					bind:value={inputValue}
					placeholder="Enter your API key to get started"
					on:keydown={handleKeydown}
				/>
				<button class="primary" on:click={saveApiKey} disabled={!inputValue.trim()}>
					Connect
				</button>
			</div>
		{:else if isEditing}
			<div class="input-group">
				<input
					type="password"
					bind:value={inputValue}
					placeholder="Enter new API key"
					on:keydown={handleKeydown}
				/>
				<button class="primary" on:click={saveApiKey} disabled={!inputValue.trim()}>Save</button>
				<button class="secondary" on:click={cancelEditing}>Cancel</button>
			</div>
		{:else}
			<div class="key-status">
				<span class="key-badge">🔑 {showKey ? apiKey : getMaskedKey(apiKey)}</span>
				<button class="icon-btn" on:click={() => showKey = !showKey}>{showKey ? '🙈' : '👁️'}</button>
				<button class="secondary small" on:click={startEditing}>Change</button>
				<button class="danger small" on:click={clearApiKey}>Remove</button>
			</div>
		{/if}
	</div>

	{#if apiKey && !isEditing}
		<div class="database-controls">
			<div class="status-row">
				{#if $dbReady}
					<span class="status ready">● Database ready</span>
					<span class="count">{totalCodeUrls.toLocaleString()} unique repositories</span>
				{:else}
					<span class="status pending">● Database not loaded</span>
				{/if}
				
				<button 
					class="primary" 
					on:click={handleLoadDatabase} 
					disabled={$isLoading}
				>
					{#if $isLoading}
						<span class="spinner"></span> Loading...
					{:else}
						🔄 {$dbReady ? 'Reload' : 'Load'} Database
					{/if}
				</button>
			</div>
			
			{#if $lastLoaded}
				<span class="last-loaded">Last loaded: {formatLastLoaded($lastLoaded)}</span>
			{/if}
			
			{#if $loadError}
				<div class="error">{$loadError}</div>
			{/if}
		</div>

		{#if $dbReady}
			{#if selectedEmailHash || selectedCountry}
				<div class="active-filters">
					{#if selectedEmailHash}
						<div class="filter-banner user-filter-banner">
							<span class="filter-banner-icon">👤</span>
							<span class="filter-banner-label">User:</span>
							<span class="filter-banner-value">{selectedUserName}</span>
							<button class="filter-banner-clear" on:click={clearUserFilter}>×</button>
						</div>
					{/if}
					{#if selectedCountry}
						<div class="filter-banner country-filter-banner">
							<span class="filter-banner-icon">🌍</span>
							<span class="filter-banner-label">Country:</span>
							<span class="filter-banner-value">{selectedCountry}</span>
							<button class="filter-banner-clear" on:click={clearCountryFilter}>×</button>
						</div>
					{/if}
				</div>
			{/if}
			<div class="filter-controls">
				<div class="filter-group ysws-filter">
					<label>Filter by YSWS:</label>
					<div class="ysws-multiselect">
						<div class="ysws-selected-tags">
							{#each selectedYsws as ysws}
								<span class="ysws-tag">
									{ysws}
									<button class="ysws-tag-remove" on:click={() => removeYsws(ysws)}>×</button>
								</span>
							{/each}
							<input
								type="text"
								bind:value={yswsSearchQuery}
								bind:this={yswsInputRef}
								placeholder={selectedYsws.length === 0 ? "Type to search YSWS..." : ""}
								on:focus={handleYswsInputFocus}
								on:blur={handleYswsInputBlur}
								on:keydown={handleYswsInputKeydown}
								class="ysws-search-input"
							/>
						</div>
						{#if showYswsDropdown && filteredYsws.length > 0}
							<div class="ysws-dropdown">
								{#each filteredYsws.slice(0, 10) as ysws}
									<button class="ysws-dropdown-item" on:mousedown={() => addYsws(ysws)}>
										{ysws}
									</button>
								{/each}
								{#if filteredYsws.length > 10}
									<div class="ysws-dropdown-more">+{filteredYsws.length - 10} more...</div>
								{/if}
							</div>
						{/if}
					</div>
				</div>
				<div class="filter-group">
					<label for="min-mentions">Min articles:</label>
					<input
						type="number"
						id="min-mentions"
						bind:value={minMentions}
						on:change={refreshCodeUrls}
						min="0"
						class="number-input"
						placeholder="0"
					/>
				</div>
				<div class="filter-group">
					<label for="sort-by">Sort by:</label>
					<select id="sort-by" bind:value={sortBy} on:change={refreshCodeUrls}>
						<option value="date">Most Recent</option>
						<option value="hours">Total Hours</option>
						<option value="mentions">Most Articles</option>
					</select>
				</div>
				<div class="filter-stats">
					<span class="stat-item">{totalCodeUrls.toLocaleString()} project{totalCodeUrls !== 1 ? 's' : ''}</span>
					<span class="stat-divider">•</span>
					<span class="stat-item weighted">{totalHours / 10 < 10 ? (totalHours / 10).toFixed(1) : Math.round(totalHours / 10).toLocaleString()} weighted</span>
				</div>
			</div>
		{/if}

		{#if $dbReady && codeUrls.length > 0}
			<div class="tree-container" bind:this={scrollContainer} on:scroll={handleScroll}>
				<div class="tree">
					{#each codeUrls as codeUrlItem (codeUrlItem.code_url ?? '__NULL__')}
						{@const codeUrlKey = codeUrlItem.code_url ?? '__NULL__'}
						{@const isExpanded = !!expandedCodeUrls[codeUrlKey]}
						{@const codeUrlData = expandedCodeUrls[codeUrlKey]}
						
						<div class="tree-node code-url-node">
							<button 
								class="tree-toggle"
								on:click={() => toggleCodeUrl(codeUrlItem.code_url)}
								aria-expanded={isExpanded}
							>
								<span class="chevron" class:expanded={isExpanded}>▶</span>
								<span class="node-icon">📁</span>
								<span class="node-label">{codeUrlItem.code_url || '(no code URL)'}</span>
								<span class="badge">{codeUrlItem.project_count} project{codeUrlItem.project_count !== 1 ? 's' : ''}</span>
								{#if codeUrlItem.article_count > 0}
									<span class="articles-badge">📰 {codeUrlItem.article_count} article{codeUrlItem.article_count !== 1 ? 's' : ''}</span>
								{/if}
								{#if codeUrlItem.total_hours}
									<span class="hours-badge">⏱️ {formatHours(codeUrlItem.total_hours)}h total</span>
								{/if}
								{#if codeUrlItem.latest_approved_at}
									<span class="date-badge">📅 {formatDate(codeUrlItem.latest_approved_at)}</span>
								{/if}
								{#each formatYswsNames(codeUrlItem.ysws_names) as yswsName}
									<button 
										class="ysws-badge clickable"
										on:click|stopPropagation={() => addYswsFromBadge(yswsName)}
										title="Filter by {yswsName}"
									>{yswsName}</button>
								{/each}
								{#if codeUrlItem.countries}
									{@const countryList = codeUrlItem.countries.split(',').map(c => c.trim()).filter(Boolean)}
									{#each [...new Set(countryList)].slice(0, 3) as country}
										<button 
											class="country clickable"
											on:click|stopPropagation={() => setCountryFilter(country)}
											title="Filter by {country}"
										>{country}</button>
									{/each}
									{#if [...new Set(countryList)].length > 3}
										<span class="country">+{[...new Set(countryList)].length - 3}</span>
									{/if}
								{/if}
							</button>
							
							{#if isExpanded && codeUrlData?.loaded}
								<div class="tree-children">
									<!-- Approved Projects (same level as articles) -->
									{#each codeUrlData.projects as project (project.record_id)}
										<div class="tree-node project-node">
											<div class="project-row">
											<div class="tree-item">
												<span class="node-icon">🚀</span>
												<span class="node-label">
													<button 
														class="user-link"
														on:click|stopPropagation={() => setUserFilter(project.email_hash, `${project.first_name || 'Unknown'}${project.last_name ? ' ' + project.last_name : ''}${project.git_hub_username ? ' (@' + project.git_hub_username + ')' : ''}`)}
														title="Filter by this user's projects"
													>
														{project.first_name || 'Unknown'}{#if project.last_name} {project.last_name}{/if}
														{#if project.git_hub_username}
															<span class="username">@{project.git_hub_username}</span>
														{/if}
													</button>
												</span>
													{#if project.ysws_name}
														<button 
															class="ysws-badge clickable"
															on:click|stopPropagation={() => addYswsFromBadge(project.ysws_name)}
															title="Filter by {project.ysws_name}"
														>{project.ysws_name}</button>
													{/if}
													{#if project.hours_spent}
														<span class="hours-badge" title={project.override_hours_spent_justification || ''}>
															⏱️ {formatHours(project.hours_spent)}h
														</span>
													{/if}
													{#if project.age_when_approved}
														<span class="age-badge">🎂 {project.age_when_approved}yo</span>
													{/if}
													{#if project.approved_at}
														<span class="date-badge">📅 {formatDate(project.approved_at)}</span>
													{/if}
													{#if project.geocoded_country}
														<button 
															class="country clickable"
															on:click|stopPropagation={() => setCountryFilter(project.geocoded_country)}
															title="Filter by {project.geocoded_country}"
														>{project.geocoded_country}</button>
													{/if}
												</div>
												<div class="project-links">
													{#if project.playable_url}
														<a href={project.playable_url} target="_blank" rel="noopener" class="link-btn">
															🎮 Play
														</a>
													{/if}
													{#if project.code_url}
														<a href={project.code_url} target="_blank" rel="noopener" class="link-btn">
															📂 Code
														</a>
													{/if}
												</div>
											</div>
														</div>
									{/each}
									
									<!-- Articles (same level as approved projects) -->
									{#each codeUrlData.articleGroups || [] as group (group.url)}
										{@const isGroupExpanded = !!expandedArticleGroups[group.url]}
										{@const badge = getEngagementBadge(group.totalEngagement)}
										
										<div class="tree-node article-node">
											<button 
												class="tree-toggle"
												on:click={() => toggleArticleGroup(group.url)}
												aria-expanded={isGroupExpanded}
											>
												<span class="chevron" class:expanded={isGroupExpanded}>▶</span>
												<span class="node-icon">📰</span>
												<span class="node-label article-title-label">
													{group.headline || formatUrl(group.url)}
												</span>
												{#if group.source}
													<span class="source-badge">{group.source}</span>
												{/if}
												{#if group.latestDate}
													<span class="date-badge">📅 {formatDate(group.latestDate)}</span>
												{/if}
												{#if group.articles.length > 1}
													<span class="mention-count">{group.articles.length} mentions</span>
												{/if}
																	{#if badge}
																		<span class="engagement-badge {badge.class}">{badge.label}</span>
																	{/if}
												<a href={group.url} target="_blank" rel="noopener" class="link-btn" on:click|stopPropagation>
													↗
												</a>
											</button>
											
											{#if isGroupExpanded}
												<div class="tree-children">
													{#each group.articles as article (article.id)}
														<div class="tree-node">
															<div class="tree-item article-mention-item">
																<span class="node-icon">📋</span>
																	{#if article.date}
																	<span class="date-badge">📅 {formatDate(article.date)}</span>
																	{/if}
																	{#if article.engagement_count}
																		<span class="engagement">
																			{article.engagement_type}: {article.engagement_count.toLocaleString()}
																		</span>
																	{/if}
																{#if article.weighted_engagement_points}
																	<span class="points">
																		{article.weighted_engagement_points.toLocaleString()} pts
																	</span>
																{/if}
																	{#if article.mentions_hack_club}
																	<span class="hc-mention">🟠 HC</span>
																{/if}
																{#if article.published_by_hack_club}
																	<span class="hc-published">🔴 Published by HC</span>
																{/if}
																{#if article.headline && article.headline !== group.headline}
																	<span class="article-alt-headline">{article.headline}</span>
																	{/if}
																</div>
															</div>
														{/each}
												</div>
											{/if}
										</div>
									{/each}
									
									{#if codeUrlData.projects.length === 0 && (!codeUrlData.articleGroups || codeUrlData.articleGroups.length === 0)}
										<div class="empty-state">
											<span>📭</span> No data found
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
					
					{#if loadedCodeUrlsCount < totalCodeUrls}
						<div class="load-more">
							{#if isLoadingMore}
								<span class="spinner"></span> Loading more...
							{:else}
								<button class="secondary" on:click={loadMoreCodeUrls}>
									Load more ({totalCodeUrls - loadedCodeUrlsCount} remaining)
								</button>
							{/if}
						</div>
					{/if}
				</div>
			</div>
		{:else if $dbReady}
			<div class="empty-main">
				<span>📭</span>
				<p>No projects found in the database</p>
			</div>
		{/if}
	{/if}
</main>

<style>
	:global(body) {
		margin: 0;
		min-height: 100vh;
		display: block;
	}

	main {
		min-height: 100vh;
		padding: 2rem;
		background: linear-gradient(135deg, #0f0f1a 0%, #1a1a2e 50%, #16213e 100%);
		color: #e0e0e0;
	}

	header {
		text-align: center;
		margin-bottom: 2rem;
	}

	h1 {
		font-family: 'JetBrains Mono', 'Fira Code', monospace;
		font-size: 2.5rem;
		font-weight: 700;
		margin: 0;
		background: linear-gradient(135deg, #00d9ff, #00ff88);
		-webkit-background-clip: text;
		-webkit-text-fill-color: transparent;
		background-clip: text;
	}

	.subtitle {
		color: #888;
		margin-top: 0.5rem;
		font-size: 1.1rem;
	}

	.api-key-section {
		margin-bottom: 1.5rem;
		padding: 1rem;
		background: rgba(255,255,255,0.03);
		border: 1px solid rgba(255,255,255,0.08);
		border-radius: 12px;
	}

	.input-group {
		display: flex;
		gap: 0.5rem;
	}

	input {
		flex: 1;
		padding: 0.75rem 1rem;
		border-radius: 8px;
		border: 1px solid rgba(255,255,255,0.15);
		background: rgba(0,0,0,0.4);
		color: #fff;
		font-size: 1rem;
	}

	input:focus {
		outline: none;
		border-color: #00d9ff;
		box-shadow: 0 0 0 2px rgba(0,217,255,0.2);
	}

	input::placeholder {
		color: rgba(255,255,255,0.4);
	}

	.key-status {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.key-badge {
		font-family: monospace;
		background: rgba(0,217,255,0.1);
		padding: 0.4rem 0.8rem;
		border-radius: 6px;
		color: #00d9ff;
	}

	button {
		padding: 0.6rem 1rem;
		border-radius: 8px;
		border: none;
		font-size: 0.9rem;
		font-weight: 500;
		cursor: pointer;
		transition: all 0.2s;
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
	}

	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	button.primary {
		background: linear-gradient(135deg, #00d9ff, #00ff88);
		color: #000;
	}

	button.primary:hover:not(:disabled) {
		transform: translateY(-1px);
		box-shadow: 0 4px 12px rgba(0,217,255,0.3);
	}

	button.secondary {
		background: rgba(255,255,255,0.1);
		color: #fff;
		border: 1px solid rgba(255,255,255,0.2);
	}

	button.secondary:hover:not(:disabled) {
		background: rgba(255,255,255,0.15);
	}

	button.danger {
		background: rgba(255,82,82,0.15);
		color: #ff5252;
		border: 1px solid rgba(255,82,82,0.3);
	}

	button.danger:hover {
		background: rgba(255,82,82,0.25);
	}

	button.small {
		padding: 0.4rem 0.7rem;
		font-size: 0.8rem;
	}

	button.icon-btn {
		background: transparent;
		padding: 0.4rem;
		font-size: 1.1rem;
	}

	.database-controls {
		margin-bottom: 1.5rem;
		padding: 1rem 1.5rem;
		background: rgba(255,255,255,0.03);
		border: 1px solid rgba(255,255,255,0.08);
		border-radius: 12px;
	}

	.status-row {
		display: flex;
		align-items: center;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.status {
		font-weight: 500;
	}

	.status.ready {
		color: #00ff88;
	}

	.status.pending {
		color: #ffaa00;
	}

	.count {
		color: #888;
		font-size: 0.9rem;
	}

	.last-loaded {
		display: block;
		margin-top: 0.75rem;
		color: #666;
		font-size: 0.85rem;
	}

	.error {
		margin-top: 1rem;
		padding: 0.75rem 1rem;
		background: rgba(255,82,82,0.15);
		border: 1px solid rgba(255,82,82,0.3);
		border-radius: 8px;
		color: #ff5252;
	}

	.filter-controls {
		display: flex;
		align-items: center;
		gap: 1.5rem;
		padding: 1rem;
		background: rgba(255,255,255,0.03);
		border: 1px solid rgba(255,255,255,0.08);
		border-radius: 8px;
		margin-bottom: 1rem;
		flex-wrap: wrap;
	}

	.filter-group {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.filter-group.ysws-filter {
		flex: 1;
		min-width: 300px;
		align-items: flex-start;
	}

	.filter-group label {
		font-size: 0.875rem;
		color: #888;
		white-space: nowrap;
		padding-top: 0.5rem;
	}

	.filter-group select {
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		border: 1px solid rgba(255,255,255,0.15);
		background: rgba(0,0,0,0.3);
		color: #e0e0e0;
		font-size: 0.875rem;
		cursor: pointer;
		min-width: 150px;
	}

	.filter-group select:focus {
		outline: none;
		border-color: #00d9ff;
		box-shadow: 0 0 0 2px rgba(0,217,255,0.2);
	}

	.number-input {
		width: 70px;
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		border: 1px solid rgba(255,255,255,0.15);
		background: rgba(0,0,0,0.3);
		color: #e0e0e0;
		font-size: 0.875rem;
		text-align: center;
	}

	.number-input:focus {
		outline: none;
		border-color: #00d9ff;
		box-shadow: 0 0 0 2px rgba(0,217,255,0.2);
	}

	.ysws-multiselect {
		position: relative;
		flex: 1;
	}

	.ysws-selected-tags {
		display: flex;
		flex-wrap: wrap;
		gap: 0.375rem;
		padding: 0.375rem;
		border: 1px solid rgba(255,255,255,0.15);
		border-radius: 6px;
		background: rgba(0,0,0,0.3);
		min-height: 38px;
		align-items: center;
	}

	.ysws-selected-tags:focus-within {
		border-color: #00d9ff;
		box-shadow: 0 0 0 2px rgba(0,217,255,0.2);
	}

	.ysws-tag {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.25rem 0.5rem;
		background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
		border-radius: 4px;
		font-size: 0.8rem;
		color: white;
	}

	.ysws-tag-remove {
		background: none;
		border: none;
		color: white;
		cursor: pointer;
		padding: 0;
		font-size: 1rem;
		line-height: 1;
		opacity: 0.7;
		transition: opacity 0.15s;
	}

	.ysws-tag-remove:hover {
		opacity: 1;
	}

	.ysws-search-input {
		flex: 1;
		min-width: 120px;
		border: none;
		background: transparent;
		color: #e0e0e0;
		font-size: 0.875rem;
		padding: 0.25rem;
		outline: none;
	}

	.ysws-search-input::placeholder {
		color: #666;
	}

	.ysws-dropdown {
		position: absolute;
		top: 100%;
		left: 0;
		right: 0;
		margin-top: 4px;
		background: #1a1a2e;
		border: 1px solid rgba(255,255,255,0.15);
		border-radius: 6px;
		max-height: 200px;
		overflow-y: auto;
		z-index: 100;
		box-shadow: 0 4px 12px rgba(0,0,0,0.3);
	}

	.ysws-dropdown-item {
		display: block;
		width: 100%;
		padding: 0.5rem 0.75rem;
		border: none;
		background: none;
		color: #e0e0e0;
		font-size: 0.875rem;
		text-align: left;
		cursor: pointer;
		transition: background 0.15s;
	}

	.ysws-dropdown-item:hover {
		background: rgba(0,217,255,0.15);
	}

	.ysws-dropdown-more {
		padding: 0.5rem 0.75rem;
		font-size: 0.8rem;
		color: #666;
		text-align: center;
	}

	.filter-stats {
		margin-left: auto;
		font-size: 0.875rem;
		color: #888;
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.stat-item {
		color: #aaa;
	}

	.stat-item.weighted {
		color: #00ff88;
		font-weight: 500;
	}

	.stat-divider {
		color: #555;
	}

	.tree-container {
		max-height: calc(100vh - 340px);
		overflow-y: auto;
		border: 1px solid rgba(255,255,255,0.08);
		border-radius: 12px;
		background: rgba(0,0,0,0.2);
	}

	.tree-container::-webkit-scrollbar {
		width: 8px;
	}

	.tree-container::-webkit-scrollbar-track {
		background: rgba(255,255,255,0.05);
	}

	.tree-container::-webkit-scrollbar-thumb {
		background: rgba(255,255,255,0.2);
		border-radius: 4px;
	}

	.tree {
		padding: 1rem;
	}

	.tree-node {
		margin-bottom: 0.25rem;
	}

	.tree-toggle {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.6rem 0.8rem;
		background: rgba(255,255,255,0.02);
		border: 1px solid transparent;
		border-radius: 8px;
		color: #e0e0e0;
		text-align: left;
		font-size: 0.9rem;
		transition: all 0.15s;
	}

	.tree-toggle:hover {
		background: rgba(255,255,255,0.06);
		border-color: rgba(255,255,255,0.1);
	}

	.chevron {
		font-size: 0.7rem;
		color: #666;
		transition: transform 0.2s;
		flex-shrink: 0;
	}

	.chevron.expanded {
		transform: rotate(90deg);
	}

	.node-icon {
		font-size: 1.1rem;
		flex-shrink: 0;
	}

	.node-label {
		flex: 1;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.username {
		color: #00d9ff;
		font-size: 0.85rem;
	}

	.user-link {
		background: none;
		border: none;
		padding: 0;
		color: inherit;
		font: inherit;
		cursor: pointer;
		text-decoration: none;
		transition: all 0.15s;
	}

	.user-link:hover {
		color: #00d9ff;
		text-decoration: underline;
	}

	.user-link .username {
		color: #00d9ff;
	}

	.active-filters {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}

	.filter-banner {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.5rem 0.75rem;
		border-radius: 6px;
		font-size: 0.85rem;
	}

	.filter-banner.user-filter-banner {
		background: linear-gradient(135deg, rgba(0,217,255,0.15), rgba(0,255,136,0.1));
		border: 1px solid rgba(0,217,255,0.3);
	}

	.filter-banner.country-filter-banner {
		background: linear-gradient(135deg, rgba(255,170,0,0.15), rgba(255,136,0,0.1));
		border: 1px solid rgba(255,170,0,0.3);
	}

	.filter-banner-icon {
		font-size: 1rem;
	}

	.filter-banner-label {
		color: #888;
	}

	.filter-banner-value {
		color: #00d9ff;
		font-weight: 500;
	}

	.country-filter-banner .filter-banner-value {
		color: #ffaa00;
	}

	.filter-banner-clear {
		background: rgba(255,82,82,0.15);
		border: 1px solid rgba(255,82,82,0.3);
		color: #ff5252;
		padding: 0.15rem 0.4rem;
		border-radius: 4px;
		font-size: 0.75rem;
		cursor: pointer;
		transition: all 0.15s;
		margin-left: 0.25rem;
	}

	.filter-banner-clear:hover {
		background: rgba(255,82,82,0.25);
	}

	.country {
		color: #888;
		font-size: 0.8rem;
		background: rgba(255,255,255,0.05);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		border: none;
	}

	.country.clickable {
		cursor: pointer;
		transition: all 0.15s;
	}

	.country.clickable:hover {
		background: rgba(255,170,0,0.2);
		color: #ffaa00;
	}

	.hours-badge {
		color: #ffaa00;
		font-size: 0.75rem;
		background: rgba(255,170,0,0.15);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		cursor: help;
	}

	.age-badge {
		color: #ff77aa;
		font-size: 0.75rem;
		background: rgba(255,119,170,0.15);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
	}

	.date-badge {
		color: #88aaff;
		font-size: 0.75rem;
		background: rgba(136,170,255,0.15);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
	}

	.ysws-badge {
		color: #b388ff;
		font-size: 0.75rem;
		background: rgba(179,136,255,0.15);
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		border: none;
	}

	.ysws-badge.clickable {
		cursor: pointer;
		transition: all 0.15s;
	}

	.ysws-badge.clickable:hover {
		background: rgba(179,136,255,0.3);
		color: #d4b8ff;
	}

	.badge {
		background: rgba(0,217,255,0.15);
		color: #00d9ff;
		padding: 0.2rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	.articles-badge {
		background: rgba(255,136,0,0.15);
		color: #ff8800;
		padding: 0.2rem 0.5rem;
		border-radius: 4px;
		font-size: 0.75rem;
		flex-shrink: 0;
	}

	.tree-children {
		margin-left: 1.5rem;
		padding-left: 1rem;
		border-left: 2px solid rgba(255,255,255,0.08);
		margin-top: 0.25rem;
	}

	.project-node {
		position: relative;
	}

	.project-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.project-row .tree-toggle {
		flex: 1;
	}

	.project-links {
		display: flex;
		gap: 0.25rem;
		flex-shrink: 0;
	}

	.link-btn {
		font-size: 0.75rem;
		padding: 0.25rem 0.5rem;
		background: rgba(255,255,255,0.05);
		border-radius: 4px;
		color: #888;
		text-decoration: none;
		transition: all 0.15s;
	}

	.link-btn:hover {
		background: rgba(0,217,255,0.2);
		color: #00d9ff;
	}

	/* Tree item (non-expandable row) */
	.tree-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.6rem 0.8rem;
		background: rgba(255,255,255,0.02);
		border: 1px solid transparent;
		border-radius: 8px;
		color: #e0e0e0;
		font-size: 0.9rem;
		flex: 1;
	}

	.tree-item:hover {
		background: rgba(255,255,255,0.04);
		border-color: rgba(255,255,255,0.08);
	}

	.project-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.article-node {
		/* Article entries styling */
	}

	.article-title-label {
		color: #fff;
	}

	.source-badge {
		font-size: 0.7rem;
		padding: 0.1rem 0.4rem;
		background: rgba(136,136,136,0.2);
		color: #aaa;
		border-radius: 3px;
		flex-shrink: 0;
	}

	.mention-count {
		font-size: 0.75rem;
		padding: 0.15rem 0.5rem;
		background: rgba(0,217,255,0.15);
		color: #00d9ff;
		border-radius: 4px;
		flex-shrink: 0;
	}

	.article-mention-item {
		font-size: 0.85rem;
		padding: 0.5rem 0.8rem;
	}

	.article-alt-headline {
		color: #888;
		font-size: 0.8rem;
		margin-left: 0.5rem;
		font-style: italic;
	}

	.points {
		color: #b388ff;
	}

	.hc-published {
		color: #ff3333;
	}

	.engagement-badge {
		font-size: 0.7rem;
		padding: 0.15rem 0.5rem;
		border-radius: 4px;
		flex-shrink: 0;
	}

	.engagement-badge.viral {
		background: rgba(255,82,82,0.2);
		color: #ff5252;
	}

	.engagement-badge.hot {
		background: rgba(255,170,0,0.2);
		color: #ffaa00;
	}

	.engagement-badge.warm {
		background: rgba(0,255,136,0.15);
		color: #00ff88;
	}

	.source {
		color: #888;
	}

	.engagement {
		color: #00d9ff;
	}

	.hc-mention {
		color: #ff6600;
	}

	.empty-state {
		padding: 1.5rem;
		text-align: center;
		color: #666;
	}

	.empty-main {
		text-align: center;
		padding: 4rem;
		color: #666;
	}

	.empty-main span {
		font-size: 3rem;
		display: block;
		margin-bottom: 1rem;
	}

	.load-more {
		padding: 1rem;
		text-align: center;
	}

	.spinner {
		width: 16px;
		height: 16px;
		border: 2px solid rgba(255,255,255,0.2);
		border-top-color: #00d9ff;
		border-radius: 50%;
		animation: spin 0.8s linear infinite;
		display: inline-block;
	}

	@keyframes spin {
		to { transform: rotate(360deg); }
	}
</style>
