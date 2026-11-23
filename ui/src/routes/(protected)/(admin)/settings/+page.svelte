<script lang="ts">
	import { brand } from '$lib/brand';
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import type {
		AuditEntry,
		AuthSession,
		ContestantAccount,
		SiteSettings,
		SystemSettings,
		User
	} from '$lib/types';
	import * as Alert from '$lib/components/ui/alert';
	import { Badge } from '$lib/components/ui/badge';
	import { Button, buttonVariants } from '$lib/components/ui/button';
	import * as Card from '$lib/components/ui/card';
	import * as Field from '$lib/components/ui/field';
	import { Input } from '$lib/components/ui/input';
	import * as Popover from '$lib/components/ui/popover';
	import { Spinner } from '$lib/components/ui/spinner';
	import * as Table from '$lib/components/ui/table';
	import Search from '@lucide/svelte/icons/search';
	import SoundSettings from './sound-settings.svelte';
	import ScoringRulesForm from '$lib/components/operator/scoring-rules-form.svelte';
	import { DEFAULT_SCORING } from '$lib/operator/scoring';
	import type { ScoringRules } from '$lib/types';

	type SettingsTab = 'general' | 'sound' | 'scoring' | 'system' | 'accounts' | 'sessions' | 'audit';
	const tabs: Array<{ value: SettingsTab; label: string }> = [
		{ value: 'general', label: 'Cài đặt chung' },
		{ value: 'sound', label: 'Âm thanh' },
		{ value: 'scoring', label: 'Luật tính điểm' },
		{ value: 'system', label: 'Hệ thống' },
		{ value: 'accounts', label: 'Tài khoản' },
		{ value: 'sessions', label: 'Phiên đăng nhập' },
		{ value: 'audit', label: 'Nhật ký kiểm toán' }
	];

	let settings = $state<SiteSettings>({
		event_name: '',
		host_name: '',
		sound: {
			enabled: true,
			master_volume: 0.8,
			music_volume: 0.8,
			effects_volume: 1,
			transition_fade_ms: 120,
			bell_sound_id: 'builtin.kd.bell'
		},
		scoring: structuredClone(DEFAULT_SCORING)
	});
	let system = $state<SystemSettings>({
		bind: '127.0.0.1',
		port: 8090,
		secure_cookies: false,
		setup_complete: true
	});
	let contestants = $state<ContestantAccount[]>([]);
	let admins = $state<User[]>([]);
	let sessions = $state<AuthSession[]>([]);
	let audit = $state<AuditEntry[]>([]);
	let contestantPasswords = $state<Record<number, string>>({});
	let adminPasswords = $state<Record<number, string>>({});
	let newAdmin = $state({ username: '', password: '' });
	let query = $state('');
	let activeTab = $state<SettingsTab>('general');
	let loading = $state(true);
	let loadingAccounts = $state(true);
	let loadingSessions = $state(true);
	let loadingAudit = $state(true);
	let loadingSystem = $state(true);
	let saving = $state(false);
	let error = $state('');
	let notice = $state('');
	let onlineUserIDs = $derived(
		new Set(sessions.filter((session) => session.online).map((session) => session.user_id))
	);

	onMount(() => {
		void api.settings
			.get()
			.then((value) => (settings = value))
			.catch(showError)
			.finally(() => (loading = false));
		void api.system
			.get()
			.then((value) => (system = value))
			.catch(showError)
			.finally(() => (loadingSystem = false));
		void loadAccounts().finally(() => (loadingAccounts = false));
		void loadSessions().finally(() => (loadingSessions = false));
		void api.admin.audit
			.list()
			.then((value) => (audit = value))
			.catch(showError)
			.finally(() => (loadingAudit = false));

		const refresh = window.setInterval(() => void loadSessions(false), 15_000);
		return () => window.clearInterval(refresh);
	});

	async function loadAccounts(): Promise<void> {
		try {
			[contestants, admins] = await Promise.all([
				api.admin.contestants.list(),
				api.admin.accounts.list()
			]);
		} catch (cause) {
			showError(cause);
		}
	}

	async function saveSystemSettings(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await run(async () => {
			const response = await api.system.update(system);
			system = response.system;
			notice = 'Đã lưu cài đặt hệ thống. Vui lòng khởi động lại ứng dụng để áp dụng địa chỉ và cổng mới.';
		});
	}
	async function saveScoring(scoring: ScoringRules): Promise<boolean> {
		let saved = false;
		await run(async () => {
			settings = await api.settings.update({ ...settings, scoring });
			notice = 'Đã lưu luật tính điểm.';
			saved = true;
		});
		return saved;
	}

	async function loadSessions(reportError = true): Promise<void> {
		try {
			sessions = await api.admin.sessions.list();
		} catch (cause) {
			if (reportError) showError(cause);
		}
	}

	async function saveSettings(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await run(async () => {
			settings = await api.settings.update(settings);
			notice = 'Đã lưu cài đặt chung.';
		});
	}

	async function resetContestantPassword(slot: number): Promise<void> {
		const password = contestantPasswords[slot] ?? '';
		if (!/^\d{6}$/.test(password)) {
			showError(new Error('Mật khẩu thí sinh phải gồm đúng 6 chữ số.'));
			return;
		}
		await run(async () => {
			await api.admin.contestants.resetPassword(slot, password);
			contestantPasswords[slot] = '';
			await Promise.all([loadAccounts(), loadSessions()]);
			notice = `Đã đổi PIN cho thi_sinh_${slot} và đăng xuất phiên cũ.`;
		});
	}

	async function createAdmin(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (newAdmin.password.length < 8) {
			showError(new Error('Mật khẩu quản trị phải có ít nhất 8 ký tự.'));
			return;
		}
		await run(async () => {
			await api.admin.accounts.create(newAdmin.username.trim(), newAdmin.password);
			newAdmin = { username: '', password: '' };
			await loadAccounts();
			notice = 'Đã tạo tài khoản quản trị.';
		});
	}

	async function resetAdminPassword(account: User): Promise<void> {
		const password = adminPasswords[account.id] ?? '';
		if (password.length < 8) {
			showError(new Error('Mật khẩu quản trị phải có ít nhất 8 ký tự.'));
			return;
		}
		await run(async () => {
			await api.admin.accounts.resetPassword(account.id, password);
			adminPasswords[account.id] = '';
			if (account.id === api.auth.user?.id) {
				await api.auth.logout().catch(() => undefined);
				window.location.hash = '#/';
				return;
			}
			await loadSessions();
			notice = `Đã đổi mật khẩu cho ${account.username} và đăng xuất các phiên cũ.`;
		});
	}

	async function setAdminEnabled(account: User): Promise<void> {
		await run(async () => {
			await api.admin.accounts.setEnabled(account.id, account.disabled);
			await Promise.all([loadAccounts(), loadSessions()]);
			notice = account.disabled
				? `Đã bật ${account.username}.`
				: `Đã vô hiệu hóa ${account.username}.`;
		});
	}

	async function invalidateSession(session: AuthSession): Promise<void> {
		await run(async () => {
			await api.admin.sessions.invalidate(session.id);
			if (session.current) {
				await api.auth.logout().catch(() => undefined);
				window.location.hash = '#/';
				return;
			}
			await loadSessions();
			notice = `Đã thu hồi phiên của ${session.username}.`;
		});
	}

	async function searchAudit(): Promise<void> {
		try {
			audit = await api.admin.audit.list(query);
		} catch (cause) {
			showError(cause);
		}
	}

	async function run(action: () => Promise<void>): Promise<void> {
		saving = true;
		error = '';
		notice = '';
		try {
			await action();
		} catch (cause) {
			showError(cause);
		} finally {
			saving = false;
		}
	}

	function showError(cause: unknown): void {
		error = cause instanceof Error ? cause.message : 'Không thể hoàn thành thao tác';
		notice = '';
	}

	function formatDate(value: string): string {
		return new Date(value).toLocaleString('vi-VN');
	}

	function sessionCount(userID: number): number {
		return sessions.filter((session) => session.user_id === userID).length;
	}
</script>

<svelte:head><title>Cài đặt - {brand.appName}</title></svelte:head>

<h1 class="text-3xl font-bold tracking-tight">Cài đặt</h1>

{#if error}
	<Alert.Root variant="destructive">
		<Alert.Title>Không thể hoàn thành</Alert.Title>
		<Alert.Description>{error}</Alert.Description>
	</Alert.Root>
{:else if notice}
	<Alert.Root>
		<Alert.Title>Thành công</Alert.Title>
		<Alert.Description>{notice}</Alert.Description>
	</Alert.Root>
{/if}

<nav class="grid grid-cols-2 rounded-lg bg-muted p-1 md:grid-cols-6" aria-label="Mục cài đặt">
	{#each tabs as tab (tab.value)}
		<button
			type="button"
			class="rounded-md px-3 py-2 text-sm font-medium transition-colors"
			class:bg-background={activeTab === tab.value}
			class:shadow-sm={activeTab === tab.value}
			class:text-muted-foreground={activeTab !== tab.value}
			aria-current={activeTab === tab.value ? 'page' : undefined}
			onclick={() => (activeTab = tab.value)}>{tab.label}</button
		>
	{/each}
</nav>

{#if activeTab === 'general'}
	<Card.Root>
		<Card.Header><Card.Title>Thông tin sự kiện</Card.Title></Card.Header>
		<Card.Content>
			{#if loading}
				<Spinner />
			{:else}
				<form onsubmit={saveSettings}>
					<Field.FieldGroup>
						<Field.Field>
							<Field.FieldLabel for="event-name">Tên sự kiện</Field.FieldLabel>
							<Input id="event-name" bind:value={settings.event_name} maxlength={120} required />
						</Field.Field>
						<Field.Field>
							<Field.FieldLabel for="host-name">Đơn vị tổ chức</Field.FieldLabel>
							<Input id="host-name" bind:value={settings.host_name} maxlength={120} />
						</Field.Field>
						<Button class="w-fit" type="submit" disabled={saving}>
							{#if saving}<Spinner data-icon="inline-start" />{/if}Lưu cài đặt
						</Button>
					</Field.FieldGroup>
				</form>
			{/if}
		</Card.Content>
	</Card.Root>
{:else if activeTab === 'sound'}
	<SoundSettings />
{:else if activeTab === 'scoring'}
	<Card.Root>
		<Card.Header>
			<Card.Title>Luật tính điểm</Card.Title>
			<Card.Description>Điểm gợi ý khi bấm Đúng/Sai. Thay đổi không làm đổi điểm đã ghi.</Card.Description>
		</Card.Header>
		<Card.Content>
			<ScoringRulesForm value={settings.scoring} {saving} onsave={saveScoring} />
		</Card.Content>
	</Card.Root>
{:else if activeTab === 'system'}
	<Card.Root>
		<Card.Header>
			<Card.Title>Cài đặt hệ thống</Card.Title>
			<Card.Description>
				Thay đổi địa chỉ hoặc cổng sẽ có hiệu lực sau khi khởi động lại ứng dụng.
			</Card.Description>
		</Card.Header>
		<Card.Content>
			{#if loadingSystem}
				<Spinner />
			{:else}
				<form onsubmit={saveSystemSettings}>
					<Field.FieldGroup>
						<Field.Field>
							<Field.FieldLabel for="system-bind">Địa chỉ lắng nghe</Field.FieldLabel>
							<select id="system-bind" class="border-input bg-background h-9 w-full rounded-md border px-3 text-sm" bind:value={system.bind}>
								<option value="127.0.0.1">127.0.0.1 (chỉ máy này)</option>
								<option value="localhost">localhost (chỉ máy này)</option>
							</select>
						</Field.Field>
						<Field.Field>
							<Field.FieldLabel for="system-port">Cổng</Field.FieldLabel>
							<Input id="system-port" type="number" min={1} max={65535} bind:value={system.port} required />
						</Field.Field>
						<label class="flex items-center gap-2 text-sm">
							<input type="checkbox" bind:checked={system.secure_cookies} />
							Cookie bảo mật (chỉ bật khi dùng HTTPS)
						</label>
						<Button class="w-fit" type="submit" disabled={saving}>
							{#if saving}<Spinner data-icon="inline-start" />{/if}Lưu cài đặt hệ thống
						</Button>
					</Field.FieldGroup>
				</form>
			{/if}
		</Card.Content>
	</Card.Root>
{:else if activeTab === 'accounts'}
	<Card.Root>
		<Card.Header>
			<Card.Title>Tài khoản quản trị</Card.Title>
		</Card.Header>
		<Card.Content class="flex flex-col gap-6">
			<form onsubmit={createAdmin}>
				<Field.FieldGroup class="grid md:grid-cols-[1fr_1fr_auto] md:items-end">
					<Field.Field>
						<Field.FieldLabel for="new-admin-username">Tên đăng nhập</Field.FieldLabel>
						<Input id="new-admin-username" bind:value={newAdmin.username} required />
					</Field.Field>
					<Field.Field>
						<Field.FieldLabel for="new-admin-password">Mật khẩu</Field.FieldLabel>
						<Input
							id="new-admin-password"
							type="password"
							minlength={8}
							bind:value={newAdmin.password}
							required
						/>
					</Field.Field>
					<Button
						type="submit"
						disabled={saving || !newAdmin.username.trim() || newAdmin.password.length < 8}
						>Tạo tài khoản</Button
					>
				</Field.FieldGroup>
			</form>

			{#if loadingAccounts}
				<Spinner />
			{:else}
				<div class="grid gap-4 md:grid-cols-2">
					{#each admins as account (account.id)}
						<div class="flex flex-col gap-4 rounded-lg border p-4">
							<div class="flex items-start justify-between gap-3">
								<div>
									<p class="font-semibold">{account.username}</p>
									<p class="text-sm text-muted-foreground">
										{sessionCount(account.id)} phiên đăng nhập
									</p>
								</div>
								<div class="flex gap-2">
									{#if onlineUserIDs.has(account.id)}<Badge>Trực tuyến</Badge>{/if}
									<Badge variant={account.disabled ? 'destructive' : 'secondary'}
										>{account.disabled ? 'Đã tắt' : 'Đang bật'}</Badge
									>
								</div>
							</div>
							<Field.Field>
								<Field.FieldLabel for={`admin-password-${account.id}`}
									>Mật khẩu mới</Field.FieldLabel
								>
								<Input
									id={`admin-password-${account.id}`}
									type="password"
									minlength={8}
									autocomplete="new-password"
									bind:value={adminPasswords[account.id]}
								/>
							</Field.Field>
							<div class="flex flex-wrap gap-2">
								<Button
									variant="outline"
									disabled={saving || (adminPasswords[account.id] ?? '').length < 8}
									onclick={() => resetAdminPassword(account)}>Đổi mật khẩu</Button
								>
								<Button
									variant={account.disabled ? 'secondary' : 'destructive'}
									disabled={saving || account.id === api.auth.user?.id}
									onclick={() => setAdminEnabled(account)}
									>{account.disabled ? 'Bật tài khoản' : 'Vô hiệu hóa'}</Button
								>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>

	<Card.Root>
		<Card.Header>
			<Card.Title>Bốn tài khoản thí sinh</Card.Title>
		</Card.Header>
		<Card.Content>
			{#if loadingAccounts}
				<Spinner />
			{:else}
				<div class="grid gap-4 md:grid-cols-2">
					{#each contestants as account (account.fixed_slot)}
						<div class="flex flex-col gap-4 rounded-lg border p-4">
							<div class="flex items-start justify-between gap-3">
								<div>
									<p class="font-semibold">{account.username}</p>
									<p class="text-sm text-muted-foreground">Vị trí {account.fixed_slot}</p>
								</div>
								<div class="flex gap-2">
									<Badge variant={onlineUserIDs.has(account.id) ? 'default' : 'outline'}
										>{onlineUserIDs.has(account.id) ? 'Trực tuyến' : 'Ngoại tuyến'}</Badge
									>
									{#if account.password_change_required}<Badge variant="destructive"
											>Cần đổi PIN</Badge
										>{/if}
								</div>
							</div>
							<Field.Field>
								<Field.FieldLabel for={`contestant-password-${account.fixed_slot}`}
									>PIN mới</Field.FieldLabel
								>
								<Input
									id={`contestant-password-${account.fixed_slot}`}
									type="password"
									inputmode="numeric"
									pattern="[0-9]{6}"
									maxlength={6}
									autocomplete="new-password"
									bind:value={contestantPasswords[account.fixed_slot]}
									placeholder="6 chữ số"
								/>
							</Field.Field>
							<Button
								variant="outline"
								disabled={saving}
								onclick={() => resetContestantPassword(account.fixed_slot)}>Đổi PIN</Button
							>
						</div>
					{/each}
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
{:else if activeTab === 'sessions'}
	<Card.Root>
		<Card.Header>
			<Card.Title>Phiên đăng nhập</Card.Title>
			<Card.Description
				>Thí sinh chỉ có một phiên; quản trị viên có thể đăng nhập nhiều thiết bị.</Card.Description
			>
		</Card.Header>
		<Card.Content>
			{#if loadingSessions}
				<Spinner />
			{:else}
				<div class="overflow-x-auto rounded-md border">
					<Table.Root>
						<Table.Header>
							<Table.Row>
								<Table.Head>Tài khoản</Table.Head>
								<Table.Head>Trạng thái</Table.Head>
								<Table.Head>IP</Table.Head>
								<Table.Head>Thiết bị / trình duyệt</Table.Head>
								<Table.Head>Đăng nhập lúc</Table.Head>
								<Table.Head>Hoạt động gần nhất</Table.Head>
								<Table.Head>Hết hạn</Table.Head>
								<Table.Head></Table.Head>
							</Table.Row>
						</Table.Header>
						<Table.Body>
							{#if sessions.length === 0}
								<Table.Row
									><Table.Cell colspan={8} class="h-24 text-center text-muted-foreground"
										>Không có phiên đăng nhập.</Table.Cell
									></Table.Row
								>
							{:else}
								{#each sessions as session (session.id)}
									<Table.Row>
										<Table.Cell>
											<div class="flex flex-col gap-1">
												<span class="font-medium">{session.username}</span>
												<div class="flex gap-1">
													<Badge variant="outline"
														>{session.role === 'admin'
															? 'BTC'
															: `Thí sinh ${session.fixed_slot}`}</Badge
													>{#if session.current}<Badge variant="secondary">Phiên này</Badge>{/if}
												</div>
											</div>
										</Table.Cell>
										<Table.Cell
											><Badge variant={session.online ? 'default' : 'outline'}
												>{session.online ? 'Trực tuyến' : 'Ngoại tuyến'}</Badge
											></Table.Cell
										>
										<Table.Cell class="font-mono text-xs">{session.ip_address || '—'}</Table.Cell>
										<Table.Cell class="max-w-80 text-xs whitespace-normal"
											>{session.user_agent || 'Không rõ'}</Table.Cell
										>
										<Table.Cell class="text-sm whitespace-nowrap"
											>{formatDate(session.created_at)}</Table.Cell
										>
										<Table.Cell class="text-sm whitespace-nowrap"
											>{formatDate(session.last_seen_at)}</Table.Cell
										>
										<Table.Cell class="text-sm whitespace-nowrap"
											>{formatDate(session.expires_at)}</Table.Cell
										>
										<Table.Cell class="text-right">
											<Popover.Root>
												<Popover.Trigger
													class={buttonVariants({ variant: 'destructive', size: 'sm' })}
													>Thu hồi</Popover.Trigger
												>
												<Popover.Content align="end">
													<Popover.Header
														><Popover.Title>Thu hồi phiên này?</Popover.Title></Popover.Header
													>
													<Popover.Description
														>{session.username} sẽ phải đăng nhập lại.</Popover.Description
													>
													<div class="flex justify-end gap-2">
														<Popover.Close
															class={buttonVariants({ variant: 'outline', size: 'sm' })}
															>Hủy</Popover.Close
														>
														<Popover.Close
															class={buttonVariants({ variant: 'destructive', size: 'sm' })}
															onclick={() => invalidateSession(session)}>Thu hồi</Popover.Close
														>
													</div>
												</Popover.Content>
											</Popover.Root>
										</Table.Cell>
									</Table.Row>
								{/each}
							{/if}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
{:else}
	<Card.Root>
		<Card.Header><Card.Title>Nhật ký kiểm toán</Card.Title></Card.Header>
		<Card.Content>
			{#if loadingAudit}
				<Spinner />
			{:else}
				<div class="mb-4 flex gap-2">
					<div class="relative flex-1">
						<Search
							class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
						/>
						<Input
							class="pl-10"
							bind:value={query}
							placeholder="Tìm hành động, loại hoặc mã đối tượng"
							onkeydown={(event) => event.key === 'Enter' && searchAudit()}
						/>
					</div>
					<Button variant="outline" onclick={searchAudit}>Tìm</Button>
				</div>
				<div class="overflow-hidden rounded-md border">
					<Table.Root>
						<Table.Header
							><Table.Row
								><Table.Head>Thời gian</Table.Head><Table.Head>Hành động</Table.Head><Table.Head
									>Đối tượng</Table.Head
								><Table.Head>Người thực hiện</Table.Head></Table.Row
							></Table.Header
						>
						<Table.Body>
							{#if audit.length === 0}
								<Table.Row
									><Table.Cell colspan={4} class="h-24 text-center text-muted-foreground"
										>Chưa có bản ghi.</Table.Cell
									></Table.Row
								>
							{:else}
								{#each audit as entry (entry.id)}
									<Table.Row
										><Table.Cell class="text-sm">{formatDate(entry.created_at)}</Table.Cell
										><Table.Cell><Badge variant="outline">{entry.action}</Badge></Table.Cell
										><Table.Cell
											>{entry.entity_type} ·
											<span class="font-mono text-xs">{entry.entity_id}</span></Table.Cell
										><Table.Cell>{entry.actor_id || 'Hệ thống'}</Table.Cell></Table.Row
									>
								{/each}
							{/if}
						</Table.Body>
					</Table.Root>
				</div>
			{/if}
		</Card.Content>
	</Card.Root>
{/if}
