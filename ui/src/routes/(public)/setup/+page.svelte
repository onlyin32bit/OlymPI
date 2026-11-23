<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import { Input } from '$lib/components/ui/input';
	import { Spinner } from '$lib/components/ui/spinner';

	let username = $state('');
	let password = $state('');
	let confirmation = $state('');
	let bind = $state('127.0.0.1');
	let port = $state(8090);
	let secureCookies = $state(false);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let complete = $state(false);

	onMount(() => {
		void api.setup
			.status()
			.then((status) => {
				if (status.configured) {
					void goto('/');
					return;
				}
				if (status.system) {
					bind = status.system.bind;
					port = status.system.port;
					secureCookies = status.system.secure_cookies;
				}
			})
			.catch(showError)
			.finally(() => (loading = false));
	});

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		error = '';
		if (password !== confirmation) {
			error = 'Mật khẩu xác nhận không khớp.';
			return;
		}
		saving = true;
		try {
			await api.setup.complete({
				username: username.trim(),
				password,
				bind,
				port: Number(port),
				secure_cookies: secureCookies
			});
			complete = true;
		} catch (cause) {
			showError(cause);
		} finally {
			saving = false;
		}
	}

	function showError(cause: unknown): void {
		error = cause instanceof Error ? cause.message : 'Không thể hoàn tất cài đặt.';
	}
</script>

<svelte:head><title>Thiết lập {brand.appName}</title></svelte:head>

<div class="mx-auto flex min-h-svh w-full max-w-2xl items-center px-4 py-10">
	<Card.Root class="w-full">
		<Card.Header>
			<Card.Title>Thiết lập {brand.appName} lần đầu</Card.Title>
			<Card.Description>
				Tạo tài khoản quản trị và nhập các thông tin bắt buộc trước khi sử dụng hệ thống.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if loading}
				<Spinner />
			{:else if complete}
				<Alert.Root>
					<Alert.Title>Đã lưu thiết lập</Alert.Title>
					<Alert.Description>
						Thiết lập đã được lưu. Vui lòng khởi động lại ứng dụng để áp dụng địa chỉ và cổng mới,
						sau đó đăng nhập bằng tài khoản quản trị.
					</Alert.Description>
				</Alert.Root>
				<Button class="mt-4" onclick={() => goto('/')}>Đến trang đăng nhập</Button>
			{:else}
				{#if error}
					<Alert.Root variant="destructive" class="mb-6">
						<Alert.Title>Không thể hoàn tất</Alert.Title>
						<Alert.Description>{error}</Alert.Description>
					</Alert.Root>
				{/if}
				<form class="space-y-6" onsubmit={submit}>
					<div class="space-y-4">
						<h2 class="font-semibold">Tài khoản quản trị</h2>
						<Field.FieldGroup class="grid gap-4 md:grid-cols-2">
							<Field.Field>
								<Field.FieldLabel for="setup-username">Tên đăng nhập</Field.FieldLabel>
								<Input id="setup-username" bind:value={username} autocomplete="username" required />
							</Field.Field>
							<Field.Field>
								<Field.FieldLabel for="setup-password">Mật khẩu</Field.FieldLabel>
								<Input id="setup-password" type="password" minlength={8} bind:value={password} required />
							</Field.Field>
						</Field.FieldGroup>
						<Field.Field>
							<Field.FieldLabel for="setup-confirmation">Xác nhận mật khẩu</Field.FieldLabel>
							<Input id="setup-confirmation" type="password" minlength={8} bind:value={confirmation} required />
						</Field.Field>
					</div>

					<Button type="submit" disabled={saving}>
						{#if saving}<Spinner data-icon="inline-start" />{/if}Lưu và hoàn tất
					</Button>
				</form>
			{/if}
		</Card.Content>
	</Card.Root>
</div>
