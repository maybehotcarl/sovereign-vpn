export const SESSION_MANAGER_ADDRESS = import.meta.env.VITE_SESSION_MANAGER || '';

export const SESSION_MANAGER_ABI = [
  {
    inputs: [
      { name: 'node', type: 'address' },
      { name: 'duration', type: 'uint256' },
    ],
    name: 'openSession',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'payable',
    type: 'function',
  },
  {
    inputs: [{ name: 'duration', type: 'uint256' }],
    name: 'calculatePrice',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'view',
    type: 'function',
  },
  {
    inputs: [{ name: 'user', type: 'address' }],
    name: 'getActiveSessionId',
    outputs: [{ name: '', type: 'uint256' }],
    stateMutability: 'view',
    type: 'function',
  },
];
