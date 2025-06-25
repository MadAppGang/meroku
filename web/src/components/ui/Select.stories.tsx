import type { Meta, StoryObj } from '@storybook/react';
import { Select } from './Select';

const meta: Meta<typeof Select> = {
  title: 'UI/Select',
  component: Select,
  parameters: {
    layout: 'padded',
  },
  tags: ['autodocs'],
};

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    children: (
      <>
        <option value="us-east-1">us-east-1</option>
        <option value="us-west-2">us-west-2</option>
        <option value="eu-west-1">eu-west-1</option>
      </>
    ),
  },
};

export const WithLabel: Story = {
  args: {
    label: 'Region',
    children: (
      <>
        <option value="us-east-1">us-east-1</option>
        <option value="us-west-2">us-west-2</option>
        <option value="eu-west-1">eu-west-1</option>
        <option value="ap-southeast-1">ap-southeast-1</option>
      </>
    ),
  },
};

export const WithOptions: Story = {
  args: {
    label: 'Memory',
    options: [
      { value: '512MB', label: '512MB' },
      { value: '1GB', label: '1GB' },
      { value: '2GB', label: '2GB' },
      { value: '4GB', label: '4GB' },
    ],
  },
};

export const Environment: Story = {
  args: {
    label: 'Environment',
    options: [
      { value: 'production', label: 'production' },
      { value: 'staging', label: 'staging' },
      { value: 'development', label: 'development' },
    ],
  },
};