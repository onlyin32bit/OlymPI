<script lang="ts">
	import { brand } from '$lib/brand';
	import { Logo } from '$lib/components/common';
	import { Button } from '$lib/components/ui/button';
	import LoginForm from './login-form.svelte';
	import { api } from '$lib/api';
	import type { UserRole } from '$lib/types';
	import { d } from '$lib/dictionary';
	import ChevronRight from '@lucide/svelte/icons/chevron-right';
	import { socket } from '$lib/socket';
	import { about } from '$lib/about';

	const baseNav = [
		{ label: 'Màn hình', href: '/display' },
		{ label: 'Quan sát', href: '/spectate' }
	] as const;
	const navigationMap: Record<UserRole, Array<{ label: string; href: string }>> = {
		admin: [
			{
				label: 'Bảng điều khiển',
				href: '/control-panel'
			},
			{
				label: 'Bảng điều khiển mới',
				href: '/control-panel-next'
			},
			{
				label: 'MC',
				href: '/master'
			},
			{
				label: 'Trận đấu',
				href: '/match'
			},
			{
				label: 'Đề thi',
				href: '/question-set'
			},
			{
				label: 'Cài đặt',
				href: '/settings'
			}
		],
		contestant: [
			{
				label: 'Màn hình thí sinh',
				href: '/contestant'
			},
			{
				label: 'Màn hình thí sinh mới',
				href: '/contestant-next'
			}
		]
	};

	let currentNav = $derived([
		...baseNav,
		...(api.auth.user ? navigationMap[api.auth.user.role] : [])
	]);
	let runtime = $derived(socket.match.current);
</script>

<svelte:head>
	<title>{api.settings.current?.event_name ?? brand.appName}</title>
</svelte:head>

<section class="flex min-h-svh items-center justify-center gap-32">
	<div class="space-y-4 bg-white">
		<div class="space-y-3">
			<Logo color="blue" class="h-32" />
			<h1 class="text-4xl font-bold">
				{api.settings.current?.event_name ?? brand.eventName}
			</h1>
			{#if runtime}
				<p class="text-muted-foreground">
					Trận đấu {runtime.match_name} - Mùa {runtime.season} - {runtime.stage}
				</p>
			{/if}
			{#if api.settings.current}
				{#if api.settings.current.host_name}
					<p class="text-sm text-muted-foreground">{api.settings.current.host_name}</p>
				{/if}
			{/if}
		</div>
		<div>
			{#if api.auth.user}
				<div class="flex flex-col items-start gap-3">
					<span>@{api.auth.user.username} - {d[api.auth.user.role]}</span>
					<Button variant="destructive" onclick={() => api.auth.logout()} class=""
						>Đăng xuất</Button
					>
				</div>
			{:else}
				<LoginForm />
			{/if}
		</div>
	</div>
	<nav
		class="grid justify-center divide-y divide-solid overflow-hidden rounded-md text-2xl font-medium"
	>
		{#each currentNav as { label, href } (href)}
			<a
				class="flex items-center gap-8 px-4 py-3 transition-colors duration-100 hover:bg-blue-100 hover:text-blue-600"
				href="#{href}">{label} <ChevronRight class="ml-auto" /></a
			>
		{/each}
	</nav>
</section>

<footer class="fixed inset-x-0 bottom-0 flex flex-wrap items-center justify-center gap-x-4 gap-y-1 p-3 text-sm text-muted-foreground">
	<a href={about.authorURL} target="_blank" rel="noreferrer" class="hover:text-foreground">{about.author}</a>
	<span aria-hidden="true">·</span>
	<a href="#/about" class="hover:text-foreground">Về {about.appName}</a>
	<span aria-hidden="true">·</span>
	<a href={about.issuesURL} target="_blank" rel="noreferrer" class="hover:text-foreground">Góp ý trên GitHub</a>
</footer>
