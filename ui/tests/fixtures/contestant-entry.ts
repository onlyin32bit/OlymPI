import { mount } from 'svelte';
import '../../src/app.css';
import Contestant from '../../src/routes/(protected)/(contestant)/contestant/+page.svelte';
import { fixture, socket, api } from './controller-state.svelte';

const target = document.getElementById('contestant-root');
if (!target) throw new Error('Missing contestant test root');
mount(Contestant, { target });
Object.assign(window, { contestantQA: { fixture, socket, api } });
