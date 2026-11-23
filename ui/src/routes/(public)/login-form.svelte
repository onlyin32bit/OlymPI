<script lang="ts">
	import { REGEXP_ONLY_DIGITS } from 'bits-ui';
	import { api } from '$lib/api';
	import { socket } from '$lib/socket';
	import * as Alert from '$lib/components/ui/alert';
	import { Button } from '$lib/components/ui/button';
	import * as Field from '$lib/components/ui/field';
	import { Input } from '$lib/components/ui/input';
	import * as InputOTP from '$lib/components/ui/input-otp';
	import * as Select from '$lib/components/ui/select';
	import * as Tabs from '$lib/components/ui/tabs';

	type LoginMode = 'contestant' | 'btc';

	const contestantSlots = [1, 2, 3, 4];

	let mode = $state<LoginMode>('contestant');
	let contestantSlot = $state('1');
	let pin = $state('');
	let username = $state('');
	let password = $state('');
	let errorMessage = $state('');
	let submitting = $state(false);
	let runtime = $derived(socket.match.current);
	let selectedContestant = $derived(
		runtime?.contestants.find((contestant) => contestant.slot === Number(contestantSlot))
	);

	function contestantLabel(slot: number): string {
		const contestant = runtime?.contestants.find((value) => value.slot === slot);
		if (!contestant) return '';
		return [contestant.name, contestant.class].filter(Boolean).join(' - ');
	}

	async function login(loginUsername: string, loginPassword: string): Promise<void> {
		errorMessage = '';
		submitting = true;
		try {
			await api.auth.login(loginUsername, loginPassword);
		} catch (cause) {
			errorMessage = cause instanceof Error ? cause.message : 'Không thể kết nối đến máy chủ';
		} finally {
			submitting = false;
		}
	}

	async function submitContestant(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await login(`thi_sinh_${contestantSlot}`, pin);
	}

	async function submitBTC(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		await login(username.trim(), password);
	}
</script>

<Tabs.Root
	value={mode}
	onValueChange={(value) => {
		mode = value as LoginMode;
		errorMessage = '';
	}}
	class="w-full gap-5"
>
	<Tabs.List class="grid h-auto w-full grid-cols-2 rounded-none bg-transparent p-0">
		<Tabs.Trigger
			value="contestant"
			class="rounded-none border-0 border-b-2 border-transparent bg-transparent py-3 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none dark:data-[state=active]:border-primary dark:data-[state=active]:bg-transparent"
		>
			Thí sinh
		</Tabs.Trigger>
		<Tabs.Trigger
			value="btc"
			class="rounded-none border-0 border-b-2 border-transparent bg-transparent py-3 shadow-none data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:shadow-none dark:data-[state=active]:border-primary dark:data-[state=active]:bg-transparent"
		>
			BTC
		</Tabs.Trigger>
	</Tabs.List>

	<Tabs.Content value="contestant">
		<form class="w-full" onsubmit={submitContestant}>
			<Field.FieldGroup>
				<Field.Field>
					<Field.FieldLabel for="contestant-slot">Vị trí thi đấu</Field.FieldLabel>
					<Select.Root type="single" bind:value={contestantSlot} disabled={submitting}>
						<Select.Trigger id="contestant-slot" class="h-auto min-h-12 w-full">
							<div class="flex min-w-0 flex-col items-start text-left">
								<span class="font-medium">Thí sinh {contestantSlot}</span>
								{#if selectedContestant}
									<span class="max-w-full truncate text-xs text-muted-foreground">
										{contestantLabel(Number(contestantSlot))}
									</span>
								{/if}
							</div>
						</Select.Trigger>
						<Select.Content>
							<Select.Group>
								{#each contestantSlots as slot (slot)}
									<Select.Item value={String(slot)} label={`Thí sinh ${slot}`}>
										<div class="flex min-w-0 flex-col items-start">
											<span class="font-medium">Thí sinh {slot}</span>
											{#if contestantLabel(slot)}
												<span class="max-w-full truncate text-xs text-muted-foreground">
													{contestantLabel(slot)}
												</span>
											{/if}
										</div>
									</Select.Item>
								{/each}
							</Select.Group>
						</Select.Content>
					</Select.Root>
				</Field.Field>

				<Field.Field data-invalid={Boolean(errorMessage)}>
					<Field.FieldLabel for="contestant-pin">Mã PIN 6 số</Field.FieldLabel>
					<InputOTP.Root
						inputId="contestant-pin"
						maxlength={6}
						pattern={REGEXP_ONLY_DIGITS}
						bind:value={pin}
						disabled={submitting}
						aria-invalid={Boolean(errorMessage)}
					>
						{#snippet children({ cells })}
							<InputOTP.Group>
								{#each cells as cell (cell)}
									<InputOTP.Slot {cell} class="size-11 text-lg" />
								{/each}
							</InputOTP.Group>
						{/snippet}
					</InputOTP.Root>
				</Field.Field>

				{#if errorMessage}
					<Alert.Root variant="destructive">
						<Alert.Title>Không thể đăng nhập</Alert.Title>
						<Alert.Description>{errorMessage}</Alert.Description>
					</Alert.Root>
				{/if}

				<Field.Field>
					<Button type="submit" disabled={submitting || pin.length !== 6}>
						{submitting ? 'Đang đăng nhập…' : 'Đăng nhập'}
					</Button>
				</Field.Field>
			</Field.FieldGroup>
		</form>
	</Tabs.Content>

	<Tabs.Content value="btc">
		<form class="w-full" onsubmit={submitBTC}>
			<Field.FieldGroup>
				<Field.Field data-invalid={Boolean(errorMessage)}>
					<Field.FieldLabel for="username">Tài khoản</Field.FieldLabel>
					<Input
						id="username"
						name="username"
						autocomplete="username"
						bind:value={username}
						aria-invalid={Boolean(errorMessage)}
						disabled={submitting}
						required
					/>
				</Field.Field>
				<Field.Field data-invalid={Boolean(errorMessage)}>
					<Field.FieldLabel for="password">Mật khẩu</Field.FieldLabel>
					<Input
						id="password"
						name="password"
						type="password"
						autocomplete="current-password"
						bind:value={password}
						aria-invalid={Boolean(errorMessage)}
						disabled={submitting}
						required
					/>
				</Field.Field>

				{#if errorMessage}
					<Alert.Root variant="destructive">
						<Alert.Title>Không thể đăng nhập</Alert.Title>
						<Alert.Description>{errorMessage}</Alert.Description>
					</Alert.Root>
				{/if}

				<Field.Field>
					<Button type="submit" disabled={submitting || !username.trim() || !password}>
						{submitting ? 'Đang đăng nhập…' : 'Đăng nhập'}
					</Button>
				</Field.Field>
			</Field.FieldGroup>
		</form>
	</Tabs.Content>
</Tabs.Root>
