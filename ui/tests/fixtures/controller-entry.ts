import { mount } from 'svelte';
import '../../src/app.css';
import OperatorConsole from '../../src/routes/(protected)/(admin)/control-panel/operator-console.svelte';
import { fixture } from './controller-state.svelte';

const target = document.getElementById('controller-root');
if (!target) throw new Error('Missing controller test root');
mount(OperatorConsole, { target });
Object.assign(window, { controllerQA: fixture });
