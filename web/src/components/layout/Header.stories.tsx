import type { Meta, StoryObj } from '@storybook/react';
import { Header } from './Header';

const meta: Meta<typeof Header> = {
  title: 'Layout/Header',
  component: Header,
  parameters: {
    layout: 'fullscreen',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Production: Story = {
  args: {
    currentEnvironment: 'production',
    onEnvironmentChange: (env) => console.log('Environment changed to:', env),
    onDeploy: () => console.log('Deploy clicked'),
  },
};

export const Staging: Story = {
  args: {
    currentEnvironment: 'staging',
    onEnvironmentChange: (env) => console.log('Environment changed to:', env),
    onDeploy: () => console.log('Deploy clicked'),
  },
};

export const Development: Story = {
  args: {
    currentEnvironment: 'development',
    onEnvironmentChange: (env) => console.log('Environment changed to:', env),
    onDeploy: () => console.log('Deploy clicked'),
  },
};