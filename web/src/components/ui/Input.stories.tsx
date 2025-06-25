import type { Meta, StoryObj } from '@storybook/react';
import { Input } from './Input';

const meta: Meta<typeof Input> = {
  title: 'UI/Input',
  component: Input,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    placeholder: 'Enter text...',
  },
};

export const WithLabel: Story = {
  args: {
    label: 'Container Image',
    placeholder: 'myapp-backend',
  },
};

export const ReadOnly: Story = {
  args: {
    label: 'Project',
    value: 'myapp-backend',
    readOnly: true,
  },
};

export const Disabled: Story = {
  args: {
    label: 'Disabled Input',
    value: 'Cannot edit',
    disabled: true,
  },
};

export const EnvironmentVariables: Story = {
  render: () => (
    <div className="flex gap-2">
      <Input placeholder="KEY" />
      <Input placeholder="VALUE" />
    </div>
  ),
};