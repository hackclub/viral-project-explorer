// See https://kit.svelte.dev/docs/types#app
// for information about these interfaces
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface Platform {}
	}
}

// Public environment variables (client-accessible)
declare module '$env/static/public' {
	export const PUBLIC_BACKEND_URL: string;
}

export {};








