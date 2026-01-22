
// Arrays of cool tech/mythology/sci-fi words for generating names
const adjectives = [
    'Quantum', 'Cyber', 'Neon', 'Spectral', 'Hyper', 'Neural',
    'Cosmic', 'Void', 'Solar', 'Lunar', 'Stellar', 'Astro',
    'Vector', 'Tensor', 'Binary', 'Digital', 'Analog', 'Kinetic',
    'Sonic', 'Prism', 'Crystal', 'Shadow', 'Iron', 'Golden',
    'Silver', 'Electric', 'Voltaic', 'Magnetic', 'Plasma', 'Fusion'
];

const nouns = [
    'Ghost', 'Spectre', 'Wraith', 'Phantom', 'Oracle', 'Prophet',
    'Titan', 'Atlas', 'Helios', 'Chronos', 'Nexus', 'Vertex',
    'Core', 'Engine', 'Brain', 'Mind', 'Spark', 'Flux',
    'Pulse', 'Wave', 'Signal', 'Code', 'Cipher', 'Rune',
    'Scribe', 'Scholar', 'Bard', 'Sage', 'Weaver', 'Architect'
];

const colors = [
    'Red', 'Blue', 'Green', 'Yellow', 'Purple', 'Orange', 'Cyan', 'Magenta', 'Teal', 'Lime', 'Indigo', 'Violet'
];

export function generateAgentName(type = 'agent') {
    const adj = adjectives[Math.floor(Math.random() * adjectives.length)];
    const noun = nouns[Math.floor(Math.random() * nouns.length)];

    if (type === 'mcp' || type === 'corvic') {
        return `Corvic ${adj} ${noun}`;
    } else if (type === 'evaluator') {
        return `${adj} Evaluator`;
    }

    return `${adj} ${noun}`;
}

const setsAdjectives = [
    'Comprehensive', 'Essential', 'Advanced', 'Core', 'Master',
    'Primary', 'Strategic', 'Tactical', 'Dynamic', 'Standard',
    'Benchmark', 'Reference', 'Baseline', 'Extended', 'Focused'
];

const setsNouns = [
    'Protocol', 'Suite', 'Matrix', 'Index', 'Catalog',
    'Battery', 'Array', 'Spectrum', 'Collection', 'Selection',
    'Logic', 'Reasoning', 'Knowledge', 'Capabilities', 'Skills'
];

export function generateQuestionSetName() {
    const adj = setsAdjectives[Math.floor(Math.random() * setsAdjectives.length)];
    const noun = setsNouns[Math.floor(Math.random() * setsNouns.length)];
    const id = Math.floor(Math.random() * 1000);
    return `${adj} ${noun} v${Math.floor(Math.random() * 5) + 1}.${Math.floor(Math.random() * 9)}`;
}
