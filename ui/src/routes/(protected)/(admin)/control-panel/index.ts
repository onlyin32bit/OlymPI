import { phaseManifest, phaseOrder, type PhaseDefinition } from '$lib/display/phase-manifest';

export type GamePhase = Pick<PhaseDefinition, 'id' | 'label'>;

export const GAME_PHASES: readonly GamePhase[] = phaseOrder.map((id) => phaseManifest[id]);
