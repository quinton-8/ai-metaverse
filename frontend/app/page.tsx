// The useChat hook needs the api property pointing to your Go backend
  const { messages, input, handleInputChange, handleSubmit, isLoading } = useChat({
    api: 'http://localhost:8080/api/chat', // <--- Add this line
    body: { modelId: activeModel }, 
  });