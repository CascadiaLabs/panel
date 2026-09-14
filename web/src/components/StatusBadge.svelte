<script>
  export let status = { running: false, inbounds: 0, outbounds: 0 };
</script>

<span class="badge {status.running ? 'green' : 'red'}">
  {status.running ? 'running' : 'stopped'}
</span>
