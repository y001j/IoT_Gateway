import { App } from 'antd';

/**
 * Message utility hook for components
 * This demonstrates the proper way to use Antd's message API
 * with the App component to avoid the static function warning
 */
export const useMessage = () => {
  const { message } = App.useApp();
  return message;
};

/**
 * Example of how to use the message API in a component:
 * 
 * import { useMessage } from '@/utils/message';
 * 
 * const MyComponent = () => {
 *   const message = useMessage();
 *   
 *   const handleClick = () => {
 *     message.success('操作成功');
 *     message.error('操作失败');
 *     message.warning('警告信息');
 *   };
 *   
 *   return <button onClick={handleClick}>Click me</button>;
 * };
 */