<script lang="ts">
	import { about } from '$lib/about';
	import ChevronLeft from '@lucide/svelte/icons/chevron-left';
	import ExternalLink from '@lucide/svelte/icons/external-link';
</script>

<svelte:head>
	<title>Về {about.appName}</title>
</svelte:head>

<section class="mx-auto flex min-h-svh max-w-xl flex-col justify-center gap-8 px-6 py-16">
	<a href="#/" class="inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
		<ChevronLeft class="size-4" /> Trang chủ
	</a>

	<header class="space-y-2">
		<h1 class="text-4xl font-bold">Về {about.appName}</h1>
		<p class="text-muted-foreground">
			Phần mềm điều khiển chương trình thi đố kiểu Đường lên đỉnh Olympia: quản lý trận đấu và đề thi,
			bảng điều khiển BTC, màn hình khán giả và trạm thí sinh, chạy từ một file duy nhất trên mạng LAN.
		</p>
	</header>

	<dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
		<dt class="text-muted-foreground">Phiên bản</dt>
		<dd>{about.version}</dd>
		<dt class="text-muted-foreground">Tác giả</dt>
		<dd>
			<a href={about.authorURL} target="_blank" rel="noreferrer" class="underline underline-offset-4"
				>{about.author}</a
			>
		</dd>
		<dt class="text-muted-foreground">Giấy phép</dt>
		<dd>{about.license}</dd>
	</dl>

	<section class="space-y-3 rounded-md border p-5">
		<h2 class="text-lg font-semibold">Góp ý và báo lỗi</h2>
		<p class="text-sm text-muted-foreground">
			Gặp lỗi hoặc có ý tưởng mới? Tạo issue trên GitHub. Nên ghi rõ phiên bản, các bước tái hiện lỗi,
			và ảnh chụp màn hình nếu có.
		</p>
		<div class="flex flex-wrap gap-3 text-sm font-medium">
			<a
				href={about.issuesURL}
				target="_blank"
				rel="noreferrer"
				class="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-primary-foreground hover:opacity-90"
				>Tạo issue <ExternalLink class="size-4" /></a
			>
			<a
				href={about.repoURL}
				target="_blank"
				rel="noreferrer"
				class="inline-flex items-center gap-1.5 rounded-md border px-4 py-2 hover:bg-accent"
				>Mã nguồn trên GitHub <ExternalLink class="size-4" /></a
			>
		</div>
	</section>
</section>
